package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB, skipped when unreachable): GET /po?supplier=<part of the supplier name> filters the list
// (and its total) by supplier name, case-insensitively.
func TestPOListSupplierFilter(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	var userID, supplierID int64
	var supplierName string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE is_active ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Skipf("users: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id, supplier_name FROM supplier WHERE length(supplier_name) >= 4 ORDER BY id LIMIT 1`).Scan(&supplierID, &supplierName); err != nil {
		t.Skipf("supplier: %v", err)
	}
	clean := func() { _, _ = pool.Exec(ctx, `DELETE FROM purchase_order WHERE po_no LIKE 'ZZ-SUP-%'`) }
	clean()
	t.Cleanup(clean)
	var poID int64
	if err := pool.QueryRow(ctx, `INSERT INTO purchase_order (po_no, job_code, created_by, updated_by, supplier_id, status)
		VALUES ('ZZ-SUP-1','MS',$1,$1,$2,'DRAFT') RETURNING id`, userID, supplierID).Scan(&poID); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: userID, Username: "zz"})
		return c.Next()
	})
	app.Get("/po", NewPOHandler(pool).List)
	list := func(supplier string) (bool, int) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", "/po?page_size=100&supplier="+url.QueryEscape(supplier), nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var r struct {
			Data struct {
				Total float64          `json:"total"`
				Data  []map[string]any `json:"data"`
			} `json:"data"`
		}
		_ = json.Unmarshal(raw, &r)
		found := false
		for _, row := range r.Data.Data {
			if v, ok := row["po_id"].(float64); ok && int64(v) == poID {
				found = true
			}
		}
		return found, int(r.Data.Total)
	}

	runes := []rune(supplierName)
	part := string(runes[1:4]) // a slice from the middle of the name
	if found, _ := list(part); !found {
		t.Errorf("supplier=%q (part of %q) did not return the PO", part, supplierName)
	}
	if found, _ := list(strings.ToUpper(part)); !found {
		t.Errorf("supplier=%q (upper case) did not return the PO", strings.ToUpper(part))
	}
	if found, total := list("zz-no-such-supplier-zz"); found || total != 0 {
		t.Errorf("an unknown supplier returned found=%v total=%d", found, total)
	}
}
