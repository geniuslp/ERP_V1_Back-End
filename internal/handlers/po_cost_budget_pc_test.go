package handlers

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Integration test against the dev database named by DATABASE_URL (or ../../.env). It commits synthetic rows
// prefixed ZZ- and deletes them afterwards (the handlers take a *pgxpool.Pool, so a rolled-back transaction is
// not possible). Skipped when no database is reachable.

func cbTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_ = godotenv.Load("../../.env")
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	// Registered first so it runs last, after every other cleanup.
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	return pool
}

func cbTestCleanup(pool *pgxpool.Pool) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM petty_cash_requisition WHERE pc_no LIKE 'ZZ-%'`) // lines cascade
	_, _ = pool.Exec(ctx, `DELETE FROM purchase_order_line WHERE po_id IN (SELECT id FROM purchase_order WHERE po_no LIKE 'ZZ-%')`)
	_, _ = pool.Exec(ctx, `DELETE FROM purchase_order WHERE po_no LIKE 'ZZ-%'`)
	_, _ = pool.Exec(ctx, `DELETE FROM project WHERE project_code LIKE 'ZZ-%'`)
}

func cbGet(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	t.Logf("GET %s -> %d %s", path, resp.StatusCode, b)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func TestCostBudgetPettyCash(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()
	cbTestCleanup(pool)
	t.Cleanup(func() { cbTestCleanup(pool) })

	// A group with one Head and several normal subgroups: s1 (PO + PC), s2 (PC only), s3 (excluded statuses only).
	var s1, s2, s3 int64
	var groupID int64
	if err := pool.QueryRow(ctx, `
		SELECT cg.id FROM cost_group cg JOIN cost_subgroup cs ON cs.group_id = cg.id
		GROUP BY cg.id
		HAVING COUNT(*) FILTER (WHERE cs.is_head) = 1 AND COUNT(*) FILTER (WHERE NOT cs.is_head AND NOT cs.is_major) >= 4
		ORDER BY cg.id LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no suitable cost group: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT id FROM cost_subgroup WHERE group_id=$1 AND NOT is_head AND NOT is_major ORDER BY id LIMIT 3`, groupID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	s1, s2, s3 = ids[0], ids[1], ids[2]

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup: %v\n%s", err, sql)
		}
	}
	pcDoc := func(pcNo, status string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO petty_cash_requisition (pc_no, pc_date, requested_by, created_by, status, doc_type)
			VALUES ($1, CURRENT_DATE, 1, 1, $2, 'PETTY_CASH') RETURNING id`, pcNo, status).Scan(&id); err != nil {
			t.Fatalf("pc header: %v", err)
		}
		return id
	}
	pcLine := func(pcID int64, no int, project string, sub *int64, amount, disc, vat float64) {
		t.Helper()
		mustExec(`INSERT INTO petty_cash_requisition_line
			(pc_id, line_no, project_code, cost_subgroup_id, description, qty, unit_price, amount, line_discount, line_vat, line_net)
			VALUES ($1,$2,$3,$4,'ZZ test line',1,$5::numeric,$5::numeric,$6::numeric,$7::numeric,$5::numeric-$6::numeric+$7::numeric)`, pcID, no, project, sub, amount, disc, vat)
	}

	// ── project A: PO on s1 (1000) + petty cash ──
	mustExec(`INSERT INTO project (project_code, project_name) VALUES ('ZZ-PC-A','ZZ project A'), ('ZZ-PC-B','ZZ project B (petty cash only)')`)
	var poID int64
	if err := pool.QueryRow(ctx, `INSERT INTO purchase_order (po_no, created_by, job_code, project_code, status)
		VALUES ('ZZ-PO-1', 1, 'MS', 'ZZ-PC-A', 'APPROVED') RETURNING id`).Scan(&poID); err != nil {
		t.Fatalf("po: %v", err)
	}
	mustExec(`INSERT INTO purchase_order_line (po_id, line_no, mat_code, qty_ordered, unit_price, cost_subgroup_id)
		VALUES ($1, 1, 'PA0100100100007', 10, 100, $2)`, poID, s1)

	// APPROVED: s1 500 - 50 discount (+31.50 VAT must NOT count) = 450.00; s2 2560; unassigned 3599.20
	d1 := pcDoc("ZZ-PC-001", "APPROVED")
	pcLine(d1, 1, "ZZ-PC-A", &s1, 500, 50, 31.5)
	d2 := pcDoc("ZZ-PC-002", "APPROVED")
	pcLine(d2, 1, "ZZ-PC-A", &s2, 2560, 0, 0)
	pcLine(d2, 2, "ZZ-PC-A", nil, 3599.20, 0, 0)
	// excluded statuses
	pcLine(pcDoc("ZZ-PC-003", "PENDING_APPROVAL"), 1, "ZZ-PC-A", &s3, 999, 0, 0)
	pcLine(pcDoc("ZZ-PC-004", "REJECTED"), 1, "ZZ-PC-A", &s3, 888, 0, 0)
	pcLine(pcDoc("ZZ-PC-005", "CANCELLED"), 1, "ZZ-PC-A", nil, 777, 0, 0)
	pcLine(pcDoc("ZZ-PC-006", "DRAFT"), 1, "ZZ-PC-A", &s3, 666, 0, 0)

	// ── project B: the UAT example, petty cash only ──
	pcLine(pcDoc("ZZ-PC-101", "APPROVED"), 1, "ZZ-PC-B", nil, 3599.20, 0, 0)
	d4 := pcDoc("ZZ-PC-102", "APPROVED")
	pcLine(d4, 1, "ZZ-PC-B", &s1, 2560, 0, 0)
	pcLine(d4, 2, "ZZ-PC-B", &s2, 35420, 0, 0)

	poH := NewPOHandler(pool)
	app := fiber.New()
	app.Get("/po/:projectCode/cost-budget-detail", poH.CostBudgetDetail)
	app.Get("/po/cost-budget/:projectCode/cost-codes/:costSubgroupId/po-lines", poH.CostBudgetPOLines)
	ovH := NewProjectOverviewHandler(pool)
	app.Get("/project-overview", ovH.List)

	type row struct {
		RowType      string  `json:"row_type"`
		SubgroupID   *int64  `json:"subgroup_id"`
		IsUnassigned bool    `json:"is_unassigned"`
		Description  *string `json:"description"`
		PoCost       float64 `json:"po_cost"`
		PcCost       float64 `json:"pc_cost"`
		PuCost       float64 `json:"pu_cost"`
		PuBal        float64 `json:"pu_bal"`
		Budget       float64 `json:"budget"`
	}
	type totals struct {
		PoCost, PcCost, PuCost, PuBal float64
	}
	detail := func(project string) ([]row, totals) {
		t.Helper()
		code, body := cbGet(t, app, "/po/"+project+"/cost-budget-detail")
		if code != 200 {
			t.Fatalf("detail %s: status %d body %v", project, code, body)
		}
		raw, _ := json.Marshal(body["data"])
		var d struct {
			Rows   []row `json:"rows"`
			Totals struct {
				PoCost float64 `json:"po_cost"`
				PcCost float64 `json:"pc_cost"`
				PuCost float64 `json:"pu_cost"`
				PuBal  float64 `json:"pu_bal"`
			} `json:"totals"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		return d.Rows, totals{d.Totals.PoCost, d.Totals.PcCost, d.Totals.PuCost, d.Totals.PuBal}
	}

	// ── project A detail ──
	rowsA, totA := detail("ZZ-PC-A")
	byType := map[string]int{}
	var sum row
	var un *row
	for i := range rowsA {
		r := rowsA[i]
		byType[r.RowType]++
		switch {
		case r.IsUnassigned:
			un = &rowsA[i]
		case r.RowType == "ITEM" && r.SubgroupID != nil && *r.SubgroupID == s1:
			if !near(r.PoCost, 1000) || !near(r.PcCost, 450) || !near(r.PuCost, 1450) {
				t.Errorf("s1 row: po=%v pc=%v pu=%v, want 1000/450/1450", r.PoCost, r.PcCost, r.PuCost)
			}
		case r.RowType == "ITEM" && r.SubgroupID != nil && *r.SubgroupID == s2:
			if !near(r.PoCost, 0) || !near(r.PcCost, 2560) || !near(r.PuCost, 2560) {
				t.Errorf("s2 row: po=%v pc=%v pu=%v, want 0/2560/2560", r.PoCost, r.PcCost, r.PuCost)
			}
		case r.RowType == "ITEM" && r.SubgroupID != nil && *r.SubgroupID == s3:
			t.Errorf("s3 (only PENDING/REJECTED/DRAFT petty cash) must not appear")
		}
		if r.RowType == "ITEM" && !r.IsUnassigned {
			sum.PoCost += r.PoCost
			sum.PcCost += r.PcCost
			sum.PuCost += r.PuCost
		}
		if r.RowType == "HEAD" || r.RowType == "MAJOR" {
			// roll-ups cover the ITEM rows only (the unassigned row has no group/job)
			if !near(r.PoCost, 1000) || !near(r.PcCost, 3010) || !near(r.PuCost, 4010) {
				t.Errorf("%s roll-up: po=%v pc=%v pu=%v, want 1000/3010/4010", r.RowType, r.PoCost, r.PcCost, r.PuCost)
			}
		}
	}
	if byType["HEAD"] != 1 || byType["MAJOR"] > 1 {
		t.Errorf("row types: %v", byType)
	}
	if un == nil {
		t.Fatalf("unassigned row missing")
	}
	if un.SubgroupID != nil || un.RowType != "ITEM" || !near(un.PcCost, 3599.20) || !near(un.PuCost, 3599.20) || !near(un.PuBal, -3599.20) ||
		un.Description == nil || *un.Description != "ไม่ระบุรหัสต้นทุน" {
		t.Errorf("unassigned row: %+v", *un)
	}
	// page total = all ITEM rows including the unassigned row
	if !near(totA.PoCost, 1000) || !near(totA.PcCost, 450+2560+3599.20) || !near(totA.PuCost, 1000+450+2560+3599.20) {
		t.Errorf("A totals: %+v", totA)
	}
	if !near(totA.PuCost, sum.PuCost+un.PuCost) || !near(totA.PcCost, sum.PcCost+un.PcCost) {
		t.Errorf("A totals %+v != sum of ITEM rows %+v", totA, sum)
	}

	// ── project B: petty cash only, UAT example ──
	rowsB, totB := detail("ZZ-PC-B")
	if len(rowsB) == 0 {
		t.Fatalf("petty-cash-only project returned an empty page")
	}
	if !near(totB.PcCost, 41579.20) || !near(totB.PuCost, 41579.20) || !near(totB.PoCost, 0) {
		t.Errorf("B totals: %+v, want pc=pu=41579.20", totB)
	}
	var projectTotal float64
	if err := pool.QueryRow(ctx, `SELECT SUM(`+pcExVatSQL+`) FROM petty_cash_requisition_line l
		JOIN petty_cash_requisition h ON h.id = l.pc_id WHERE h.status='APPROVED' AND l.project_code='ZZ-PC-B'`).Scan(&projectTotal); err != nil {
		t.Fatal(err)
	}
	if !near(projectTotal, totB.PcCost) {
		t.Errorf("project petty cash %v != page pc_cost %v", projectTotal, totB.PcCost)
	}

	// ── drill-down ──
	type line struct {
		Source   string  `json:"source"`
		POID     int64   `json:"po_id"`
		PONo     string  `json:"po_no"`
		LineNo   int     `json:"line_no"`
		Status   string  `json:"po_status"`
		Amount   float64 `json:"amount"`
		ItemName *string `json:"item_name"`
	}
	drill := func(project, sub string) (int, []line) {
		t.Helper()
		code, body := cbGet(t, app, "/po/cost-budget/"+project+"/cost-codes/"+sub+"/po-lines")
		var ls []line
		if code == 200 {
			raw, _ := json.Marshal(body["data"])
			_ = json.Unmarshal(raw, &ls)
		}
		return code, ls
	}
	itoa := func(v int64) string { b, _ := json.Marshal(v); return string(b) }

	code, ls := drill("ZZ-PC-A", itoa(s1))
	if code != 200 || len(ls) != 2 || ls[0].Source != "PO" || ls[0].PONo != "ZZ-PO-1" || !near(ls[0].Amount, 1000) ||
		ls[1].Source != "PC" || ls[1].PONo != "ZZ-PC-001" || ls[1].Status != "APPROVED" || !near(ls[1].Amount, 450) || ls[1].ItemName == nil {
		t.Errorf("drill s1: code=%d lines=%+v", code, ls)
	}
	code, ls = drill("ZZ-PC-A", itoa(s2))
	if code != 200 || len(ls) != 1 || ls[0].Source != "PC" || !near(ls[0].Amount, 2560) {
		t.Errorf("drill s2 (petty cash only): code=%d lines=%+v", code, ls)
	}
	code, ls = drill("ZZ-PC-A", "none")
	if code != 200 || len(ls) != 1 || ls[0].Source != "PC" || !near(ls[0].Amount, 3599.20) {
		t.Errorf("drill none: code=%d lines=%+v", code, ls)
	}
	code, ls = drill("ZZ-PC-B", "none")
	if code != 200 || len(ls) != 1 || !near(ls[0].Amount, 3599.20) {
		t.Errorf("drill B none: code=%d lines=%+v", code, ls)
	}
	if code, _ = drill("ZZ-PC-A", itoa(s3)); code != 404 {
		t.Errorf("drill s3 (excluded statuses only): got %d, want 404", code)
	}
	if code, _ = drill("ZZ-PC-A", "abc"); code != 400 {
		t.Errorf("drill abc: got %d, want 400", code)
	}
	if code, _ = drill("2026-XXX-004", "none"); code != 404 {
		t.Errorf("drill none on a project without unassigned petty cash: got %d, want 404", code)
	}

	// ── PO-only project unchanged: 2026-XXX-004, subgroup 2132 = 13,000 ──
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code='2026-XXX-004')`).Scan(&exists)
	if exists {
		rows4, _ := detail("2026-XXX-004")
		found := false
		for _, r := range rows4 {
			if r.SubgroupID != nil && *r.SubgroupID == 2132 {
				found = true
				if !near(r.PuCost, 13000) || !near(r.PoCost, 13000) || !near(r.PcCost, 0) {
					t.Errorf("2026-XXX-004 / 2132: po=%v pc=%v pu=%v, want 13000/0/13000", r.PoCost, r.PcCost, r.PuCost)
				}
			}
			if r.IsUnassigned {
				t.Errorf("2026-XXX-004 must not get an unassigned row")
			}
		}
		if !found {
			t.Errorf("2026-XXX-004: subgroup 2132 row missing")
		}
	}

	// ── project overview: petty cash on the ex-VAT basis ──
	code, body := cbGet(t, app, "/project-overview?search=ZZ-PC-A")
	if code != 200 {
		t.Fatalf("overview: %d %v", code, body)
	}
	raw, _ := json.Marshal(body["data"])
	var ov struct {
		Data []struct {
			ProjectCode string `json:"project_code"`
			PuBreakdown struct {
				PC float64 `json:"petty_cash"`
			} `json:"pu_breakdown"`
			AcBreakdown struct {
				PC float64 `json:"petty_cash"`
			} `json:"ac_breakdown"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &ov)
	if len(ov.Data) != 1 || !near(ov.Data[0].PuBreakdown.PC, 450+2560+3599.20) || !near(ov.Data[0].AcBreakdown.PC, 450+2560+3599.20) {
		t.Errorf("overview petty cash (ex-VAT): %+v, want 6609.20", ov.Data)
	}
}
