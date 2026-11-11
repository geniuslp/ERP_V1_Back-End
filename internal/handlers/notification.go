package handlers

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"erp-api/internal/auth"
	"erp-api/internal/config"
	"erp-api/internal/middleware"
	"erp-api/internal/notify"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationHandler serves the bell: REST list / mark-read, and the WebSocket that pushes new rows live
// (see internal/notify and migrations/053_notification.sql).
type NotificationHandler struct {
	db   *pgxpool.Pool
	cfg  *config.Config
	Hub  *notify.Hub
	auth *auth.Service
}

func NewNotificationHandler(db *pgxpool.Pool, cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{db: db, cfg: cfg, Hub: notify.NewHub(), auth: auth.NewService(cfg)}
}

// StartListener begins the pg LISTEN loop in the background.
func (h *NotificationHandler) StartListener() {
	go h.Hub.Listen(context.Background(), h.cfg.DatabaseURL, h.db)
}

// List godoc
// @Summary      My notifications (newest first)
// @Tags         Notifications
// @Security     BearerAuth
// @Param        unread_only query bool false "only unread"
// @Param        limit       query int  false "default 30, max 100"
// @Router       /notifications [get]
func (h *NotificationHandler) List(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	limit := c.QueryInt("limit", 30)
	if limit < 1 || limit > 100 {
		limit = 30
	}
	// A result notification (approved / rejected / payable) disappears once read; only "waiting for approval" rows
	// outlive being read (until the document is decided, see migration 054).
	q := `SELECT ` + notify.Columns + ` FROM notification n WHERE n.user_id = $1 AND (n.type = 'APPROVAL_REQUEST' OR n.is_read = false)`
	if c.QueryBool("unread_only", false) {
		q += ` AND (n.is_read = false OR ` + notify.PendingExpr + `)`
	}
	q += ` ORDER BY n.created_at DESC, n.id DESC LIMIT $2`
	rows, err := h.db.Query(context.Background(), q, claims.UserID, limit)
	if err != nil {
		log.Printf("notification list: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดการแจ้งเตือนได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()
	items := []notify.Item{}
	for rows.Next() {
		var it notify.Item
		if err := rows.Scan(&it.ID, &it.Type, &it.Title, &it.Message, &it.DocType, &it.DocID, &it.DocNo, &it.Link,
			&it.IsRead, &it.CreatedAt, &it.ReadAt, &it.Pending); err != nil {
			log.Printf("notification scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดการแจ้งเตือนได้ กรุณาลองใหม่อีกครั้ง")
		}
		items = append(items, it)
	}
	var unread int
	_ = h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM notification WHERE user_id=$1 AND is_read=false`,
		claims.UserID).Scan(&unread)
	return c.JSON(fiber.Map{"success": true, "data": items, "unread": unread})
}

// MarkRead godoc
// @Summary      Mark one notification as read
// @Tags         Notifications
// @Security     BearerAuth
// @Router       /notifications/{id}/read [put]
func (h *NotificationHandler) MarkRead(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id ไม่ถูกต้อง")
	}
	// Reading removes a result notification; an approval request is only marked read (it stays until decided).
	if _, err := h.db.Exec(context.Background(), `DELETE FROM notification WHERE id=$1 AND user_id=$2 AND type <> 'APPROVAL_REQUEST'`,
		id, claims.UserID); err == nil {
		_, err = h.db.Exec(context.Background(),
			`UPDATE notification SET is_read=true, read_at=NOW() WHERE id=$1 AND user_id=$2 AND type = 'APPROVAL_REQUEST' AND is_read=false`,
			id, claims.UserID)
		if err != nil {
			log.Printf("notification read: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "บันทึกไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
		}
	} else {
		log.Printf("notification read: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "บันทึกไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
	}
	return c.JSON(fiber.Map{"success": true})
}

// MarkAllRead godoc
// @Summary      Mark all my notifications as read
// @Tags         Notifications
// @Security     BearerAuth
// @Router       /notifications/read-all [put]
func (h *NotificationHandler) MarkAllRead(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	if _, err := h.db.Exec(context.Background(), `DELETE FROM notification WHERE user_id=$1 AND type <> 'APPROVAL_REQUEST'`, claims.UserID); err != nil {
		log.Printf("notification read-all: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "บันทึกไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
	}
	if _, err := h.db.Exec(context.Background(),
		`UPDATE notification SET is_read=true, read_at=NOW() WHERE user_id=$1 AND type = 'APPROVAL_REQUEST' AND is_read=false`, claims.UserID); err != nil {
		log.Printf("notification read-all: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "บันทึกไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
	}
	return c.JSON(fiber.Map{"success": true})
}

// wsConn serialises writes to one socket (a connection allows a single concurrent writer).
type wsConn struct {
	mu sync.Mutex
	c  *websocket.Conn
}

func (w *wsConn) Send(b []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.c.WriteMessage(websocket.TextMessage, b)
}

// WSAuth validates the access token for the upgrade request. Browsers cannot set an Authorization header on a
// WebSocket, so the token comes in the query string (?token=...).
func (h *NotificationHandler) WSAuth(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	claims, err := h.auth.ValidateAccessToken(c.Query("token"))
	if err != nil || claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
	}
	c.Locals("ws_user_id", claims.UserID)
	return c.Next()
}

// WS is the live channel. The server pings every 30s and drops a client that stops answering; the client only
// needs to keep the connection open (anything it sends is ignored).
func (h *NotificationHandler) WS() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		userID, _ := c.Locals("ws_user_id").(int64)
		if userID == 0 {
			_ = c.Close()
			return
		}
		conn := &wsConn{c: c}
		h.Hub.Add(userID, conn)
		defer func() { h.Hub.Remove(userID, conn); _ = c.Close() }()

		const idle = 70 * time.Second
		_ = c.SetReadDeadline(time.Now().Add(idle))
		c.SetPongHandler(func(string) error { return c.SetReadDeadline(time.Now().Add(idle)) })

		done := make(chan struct{})
		defer close(done)
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					conn.mu.Lock()
					err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
					conn.mu.Unlock()
					if err != nil {
						return
					}
				}
			}
		}()

		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
			_ = c.SetReadDeadline(time.Now().Add(idle))
		}
	})
}
