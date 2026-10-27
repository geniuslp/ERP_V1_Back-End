package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB, skipped when unreachable): warehouse stock upload, Update and Add.
// Touches one existing FAC-S stock item; qty is restored and the BULK_UPLOAD movements are deleted afterwards.
func TestWarehouseUploadUpdateAndAdd(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	var itemID int64
	var mat string
	var qty0 float64
	if err := pool.QueryRow(ctx, `SELECT id, mat_code, qty FROM stock_item WHERE warehouse_code='FAC-S' ORDER BY id LIMIT 1`).Scan(&itemID, &mat, &qty0); err != nil {
		t.Skipf("no FAC-S stock item fixture: %v", err)
	}
	var otherWH string
	_ = pool.QueryRow(ctx, `SELECT mat_code FROM stock_item WHERE warehouse_code<>'FAC-S' ORDER BY id LIMIT 1`).Scan(&otherWH)

	clean := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM stock_transaction WHERE ref_doc_type='BULK_UPLOAD' AND item_id=$1`, itemID)
		_, _ = pool.Exec(ctx, `UPDATE stock_item SET qty=$2 WHERE id=$1`, itemID, qty0)
	}
	clean()
	t.Cleanup(clean)

	h := NewStockItemHandler(pool)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: 1})
		return c.Next()
	})
	app.Post("/preview", h.PreviewWarehouseUpload)
	app.Post("/apply", h.ApplyWarehouseUpload)

	xlsxFile := func(rows [][3]any) []byte { // {mat_code, qty, unit}
		f := excelize.NewFile()
		for i, r := range rows {
			rn := i + 4 // 3 header rows
			_ = f.SetCellValue("Sheet1", "C"+strconv.Itoa(rn), "test")
			_ = f.SetCellValue("Sheet1", "D"+strconv.Itoa(rn), r[1])
			_ = f.SetCellValue("Sheet1", "E"+strconv.Itoa(rn), r[2])
			_ = f.SetCellValue("Sheet1", "G"+strconv.Itoa(rn), r[0])
		}
		buf, _ := f.WriteToBuffer()
		return buf.Bytes()
	}
	call := func(path, mode, wh string, file []byte) (int, map[string]any) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("mode", mode)
		_ = mw.WriteField("warehouse_code", wh)
		fw, _ := mw.CreateFormFile("file", "x.xlsx")
		_, _ = io.Copy(fw, bytes.NewReader(file))
		_ = mw.Close()
		req := httptest.NewRequest("POST", path, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	qtyNow := func() float64 {
		var q float64
		_ = pool.QueryRow(ctx, `SELECT qty FROM stock_item WHERE id=$1`, itemID).Scan(&q)
		return q
	}

	// Add: +5 on top
	if code, out := call("/apply", "add", "FAC-S", xlsxFile([][3]any{{mat, 5, "ea"}})); code != 200 {
		t.Fatalf("add: %d %v", code, out)
	}
	if got := qtyNow(); got != qty0+5 {
		t.Errorf("add: qty %v, want %v", got, qty0+5)
	}
	// Update: overwrite with 12 (a decrease would also be fine)
	if code, out := call("/apply", "update", "FAC-S", xlsxFile([][3]any{{mat, 12, "ea"}})); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if got := qtyNow(); got != 12 {
		t.Errorf("update: qty %v, want 12", got)
	}
	var txns int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM stock_transaction WHERE ref_doc_type='BULK_UPLOAD' AND item_id=$1`, itemID).Scan(&txns)
	if txns != 2 {
		t.Errorf("movements = %d, want 2", txns)
	}

	// all-or-nothing: one bad row (unknown code) rejects the whole file, nothing is added
	before := qtyNow()
	if code, _ := call("/apply", "add", "FAC-S", xlsxFile([][3]any{{mat, 3, "ea"}, {"ZZ-NOPE", 1, "ea"}})); code != 400 {
		t.Errorf("bad row: status %d, want 400", code)
	}
	if qtyNow() != before {
		t.Errorf("a rejected upload changed the quantity")
	}
	// an item stocked in another warehouse is rejected, not moved
	if otherWH != "" {
		if code, _ := call("/apply", "add", "FAC-S", xlsxFile([][3]any{{otherWH, 1, "ea"}})); code != 400 {
			t.Errorf("other warehouse: status %d, want 400", code)
		}
	}
	// the same code twice in one file is rejected
	if code, _ := call("/apply", "add", "FAC-S", xlsxFile([][3]any{{mat, 1, "ea"}, {mat, 1, "ea"}})); code != 400 {
		t.Errorf("duplicate: status %d, want 400", code)
	}
	// preview writes nothing and reports can_apply
	if code, out := call("/preview", "add", "FAC-S", xlsxFile([][3]any{{mat, 2, "ea"}})); code != 200 || out["data"].(map[string]any)["can_apply"] != true {
		t.Errorf("preview: %d %v", code, out)
	}
	if qtyNow() != before {
		t.Errorf("preview changed the quantity")
	}
}

// Integration test (dev DB, skipped when unreachable): the warehouse list endpoint.
func TestListWarehouseItems(t *testing.T) {
	pool := cbTestPool(t)
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM stock_item WHERE warehouse_code='FAC-S'`).Scan(&n); err != nil || n == 0 {
		t.Skip("no FAC-S stock fixture")
	}
	h := NewStockByProjectHandler(pool)
	app := fiber.New()
	app.Get("/items", h.ListWarehouseItems)
	get := func(q string) (int, map[string]any) {
		resp, err := app.Test(httptest.NewRequest("GET", "/items"+q, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, _ := get(""); code != 400 {
		t.Errorf("no warehouse: %d, want 400", code)
	}
	if code, _ := get("?warehouse_code=ZZ-NOPE"); code != 400 {
		t.Errorf("unknown warehouse: %d, want 400", code)
	}
	code, out := get("?warehouse_code=FAC-S&page_size=5")
	if code != 200 {
		t.Fatalf("list: %d %v", code, out)
	}
	d := out["data"].(map[string]any)
	if int(d["total"].(float64)) != n || len(d["data"].([]any)) != 5 {
		t.Errorf("total/page rows = %v/%d, want %d/5", d["total"], len(d["data"].([]any)), n)
	}
}
