package notify

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pusher sends Web Push messages (phone / desktop notifications while the ERP page is closed). It reads the
// subscriptions saved in push_subscription (migrations/057_push_subscription.sql).
type Pusher struct {
	db      *pgxpool.Pool
	pub     string
	priv    string
	subject string
}

// NewPusher returns nil when the VAPID keys are not configured (push is then simply off).
func NewPusher(db *pgxpool.Pool, pub, priv, subject string) *Pusher {
	if pub == "" || priv == "" {
		return nil
	}
	return &Pusher{db: db, pub: pub, priv: priv, subject: subject}
}

// PublicKey is what the browser needs to subscribe.
func (p *Pusher) PublicKey() string { return p.pub }

type pushPayload struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Link  string `json:"link"` // app-relative (e.g. /pr/12); the service worker prefixes the app base path
	Tag   string `json:"tag"`  // one notification per document: a newer one replaces the older
}

type subscription struct {
	id       int64
	endpoint string
	p256dh   string
	auth     string
}

// Send pushes one notification to every device the user subscribed. Dead subscriptions (404 / 410) are deleted.
func (p *Pusher) Send(ctx context.Context, userID int64, it Item) {
	rows, err := p.db.Query(ctx, `SELECT id, endpoint, p256dh, auth FROM push_subscription WHERE user_id = $1`, userID)
	if err != nil {
		log.Printf("push load subscriptions: %v", err)
		return
	}
	var subs []subscription
	for rows.Next() {
		var s subscription
		if err := rows.Scan(&s.id, &s.endpoint, &s.p256dh, &s.auth); err != nil {
			log.Printf("push scan: %v", err)
			continue
		}
		subs = append(subs, s)
	}
	rows.Close()
	if len(subs) == 0 {
		return
	}

	pl := pushPayload{ID: it.ID, Title: it.Title, Body: it.Message}
	if it.Link != nil {
		pl.Link = *it.Link
	}
	if it.DocType != nil && it.DocID != nil {
		pl.Tag = *it.DocType + ":" + itoa(*it.DocID)
	}
	body, _ := json.Marshal(pl)

	var wg sync.WaitGroup
	for _, s := range subs {
		wg.Add(1)
		go func(s subscription) {
			defer wg.Done()
			resp, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
				Endpoint: s.endpoint,
				Keys:     webpush.Keys{P256dh: s.p256dh, Auth: s.auth},
			}, &webpush.Options{
				Subscriber:      p.subject,
				VAPIDPublicKey:  p.pub,
				VAPIDPrivateKey: p.priv,
				TTL:             24 * 60 * 60,
				Urgency:         webpush.UrgencyHigh,
			})
			if err != nil {
				log.Printf("push send (sub %d): %v", s.id, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
				if _, err := p.db.Exec(context.Background(), `DELETE FROM push_subscription WHERE id = $1`, s.id); err != nil {
					log.Printf("push delete expired sub %d: %v", s.id, err)
				}
				return
			}
			if resp.StatusCode >= 300 {
				log.Printf("push send (sub %d): push service answered %d", s.id, resp.StatusCode)
			}
		}(s)
	}
	wg.Wait()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
