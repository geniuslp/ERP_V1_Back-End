// Package notify pushes rows of the notification table to the browsers over WebSocket.
//
// The rows are written by a database trigger on approval_request (migrations/053_notification.sql), which also runs
// pg_notify('erp_notification', {"id":..,"user_id":..}). Listen holds one dedicated connection that LISTENs on that
// channel and hands every payload to the Hub, which writes it to each open socket of that user.
package notify

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Channel = "erp_notification"

// Item is the JSON shape of a notification, used by both the REST list and the WebSocket push.
type Item struct {
	ID        int64      `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	DocType   *string    `json:"doc_type"`
	DocID     *int64     `json:"doc_id"`
	DocNo     *string    `json:"doc_no"`
	Link      *string    `json:"link"`
	IsRead    bool       `json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
	// Pending: an APPROVAL_REQUEST whose document is still waiting for this user. It stays highlighted and keeps
	// counting on the bell, read or not, until the document is approved / rejected.
	Pending bool `json:"pending"`
}

// Sender is the part of a socket the hub needs (kept abstract so the hub has no websocket dependency).
type Sender interface {
	Send(payload []byte) error
}

type Hub struct {
	mu    sync.RWMutex
	users map[int64]map[Sender]struct{}
}

func NewHub() *Hub { return &Hub{users: map[int64]map[Sender]struct{}{}} }

func (h *Hub) Add(userID int64, s Sender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.users[userID] == nil {
		h.users[userID] = map[Sender]struct{}{}
	}
	h.users[userID][s] = struct{}{}
}

func (h *Hub) Remove(userID int64, s Sender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.users[userID]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.users, userID)
		}
	}
}

// Publish sends payload to every open socket of the user; a socket that fails is dropped.
func (h *Hub) Publish(userID int64, payload []byte) {
	h.mu.RLock()
	targets := make([]Sender, 0, len(h.users[userID]))
	for s := range h.users[userID] {
		targets = append(targets, s)
	}
	h.mu.RUnlock()
	for _, s := range targets {
		if err := s.Send(payload); err != nil {
			h.Remove(userID, s)
		}
	}
}

// PendingExpr (alias n = notification) is true only for the newest APPROVAL_REQUEST of a document that still has a
// PENDING approval_request assigned to the notification's user.
const PendingExpr = `(n.type = 'APPROVAL_REQUEST'
	AND EXISTS (SELECT 1 FROM approval_request ar WHERE ar.doc_type = n.doc_type AND ar.doc_id = n.doc_id
	            AND ar.assigned_to = n.user_id AND ar.status = 'PENDING')
	AND n.id = (SELECT MAX(x.id) FROM notification x WHERE x.user_id = n.user_id AND x.type = 'APPROVAL_REQUEST'
	            AND x.doc_type = n.doc_type AND x.doc_id = n.doc_id))`

// Columns is the column list scanned into Item (table alias n).
const Columns = `n.id, n.type, n.title, COALESCE(n.message,''), n.doc_type, n.doc_id, n.doc_no, n.link, n.is_read,
	n.created_at, n.read_at, ` + PendingExpr

const selectItem = `SELECT ` + Columns + ` FROM notification n WHERE n.id = $1`

// Listen runs until ctx ends: LISTEN on Channel with a dedicated connection (reconnecting after any failure) and
// publish each new notification to its user.
func (h *Hub) Listen(ctx context.Context, dsn string, pool *pgxpool.Pool) {
	for ctx.Err() == nil {
		if err := h.listenOnce(ctx, dsn, pool); err != nil && ctx.Err() == nil {
			log.Printf("notify listener: %v (retry in 5s)", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (h *Hub) listenOnce(ctx context.Context, dsn string, pool *pgxpool.Pool) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	log.Printf("notify listener: LISTEN %s", Channel)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var p struct {
			Action string `json:"action"`
			ID     int64  `json:"id"`
			UserID int64  `json:"user_id"`
		}
		if json.Unmarshal([]byte(n.Payload), &p) != nil || p.ID == 0 {
			continue
		}
		if p.Action == "remove" { // the row was deleted (a decided approval request): tell the open bells to drop it
			rb, _ := json.Marshal(map[string]any{"event": "remove", "id": p.ID})
			h.Publish(p.UserID, rb)
			continue
		}
		var it Item
		if err := pool.QueryRow(ctx, selectItem, p.ID).Scan(&it.ID, &it.Type, &it.Title, &it.Message, &it.DocType,
			&it.DocID, &it.DocNo, &it.Link, &it.IsRead, &it.CreatedAt, &it.ReadAt, &it.Pending); err != nil {
			log.Printf("notify load %d: %v", p.ID, err)
			continue
		}
		b, _ := json.Marshal(map[string]any{"event": "notification", "data": it})
		h.Publish(p.UserID, b)
	}
}
