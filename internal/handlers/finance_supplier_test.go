package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB, skipped when unreachable): GET /finance/payments returns the supplier (ร้านค้า) name of
// PO rows (supplier master) and WO rows (supplier master, else the typed name); the single-document GET does too.
func TestFinancePaymentsSupplierName(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	var userID, supplierID int64
	var supplierName string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE is_active ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Skipf("users: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id, supplier_name FROM supplier ORDER BY id LIMIT 1`).Scan(&supplierID, &supplierName); err != nil {
		t.Skipf("supplier: %v", err)
	}
	clean := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM purchase_order WHERE po_no LIKE 'ZZ-FIN-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM work_order WHERE wo_no LIKE 'ZZ-FIN-%'`)
	}
	clean()
	t.Cleanup(clean)

	var poID, woID int64
	if err := pool.QueryRow(ctx, `INSERT INTO purchase_order (po_no, job_code, created_by, updated_by, supplier_id, status, net_amount)
		VALUES ('ZZ-FIN-PO','MS',$1,$1,$2,'PENDING_APPROVAL',100) RETURNING id`, userID, supplierID).Scan(&poID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO work_order (wo_no, work_system, employer_name, supplier_name, created_by, updated_by, status, net_amount)
		VALUES ('ZZ-FIN-WO', ARRAY['MS'],'ZZ','ร้านทดสอบ ZZ',$1,$1,'APPROVED',100) RETURNING id`, userID).Scan(&woID); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: userID, Username: "zz"})
		return c.Next()
	})
	h := NewFinanceHandler(pool)
	app.Get("/finance/payments", h.ListPayableDocs)
	app.Get("/finance/payments/:doc_type/:doc_id", h.GetPayableDoc)

	get := func(path string) map[string]any {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		return m
	}
	// An APPROVED PO only shows up once goods were received, so the test PO stays PENDING_APPROVAL.
	listRow := func(docType, status string, id int64) map[string]any {
		t.Helper()
		m := get("/finance/payments?doc_type=" + docType + "&page_size=100&status=" + status)
		d, _ := m["data"].(map[string]any)
		arr, _ := d["data"].([]any)
		for _, it := range arr {
			row, _ := it.(map[string]any)
			if v, ok := row["id"].(float64); ok && int64(v) == id {
				return row
			}
		}
		t.Fatalf("%s row %d not in the list: %v", docType, id, m)
		return nil
	}

	if row := listRow("PO", "PENDING_APPROVAL", poID); row["supplier_name"] != supplierName {
		t.Errorf("PO list supplier_name = %v, want %q", row["supplier_name"], supplierName)
	}
	if row := listRow("WO", "APPROVED", woID); row["supplier_name"] != "ร้านทดสอบ ZZ" {
		t.Errorf("WO list supplier_name = %v, want the WO's own name", row["supplier_name"])
	}
	one := get("/finance/payments/PO/" + strconv.FormatInt(poID, 10))
	if d, _ := one["data"].(map[string]any); d["supplier_name"] != supplierName {
		t.Errorf("single PO supplier_name = %v, want %q", d["supplier_name"], supplierName)
	}
}
