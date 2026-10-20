package handlers

import (
	"context"
	"log"
	"strconv"

	"erp-api/internal/models"

	"github.com/jackc/pgx/v5"
)

// projectUsageSource is one kind of "work" document that can reference a project_code. Each
// entry's selects must return exactly one column (the referenced project_code).
type projectUsageSource struct {
	label   string   // Thai doc name shown in in_use_by
	selects []string // SELECT <project_code col> FROM <table> WHERE <col> = ANY($1)
}

// projectUsageSources is the single list of tables that make a project_code "in use". Order =
// display order of in_use_by. Project SETUP tables (project_cost_budget, project_job_share,
// project_responsible_person) are deliberately not here. Column names verified against
// information_schema (dev, 2026-10-20).
var projectUsageSources = []projectUsageSource{
	{"ใบขอซื้อ (PR)", []string{`SELECT project_code FROM purchase_request WHERE project_code = ANY($1)`}},
	{"ใบสั่งซื้อ (PO)", []string{`SELECT project_code FROM purchase_order WHERE project_code = ANY($1)`}},
	{"หนังสือสั่งจ้าง (WO)", []string{`SELECT project_code FROM work_order WHERE project_code = ANY($1)`}},
	{"Memo", []string{`SELECT project_code FROM memo WHERE project_code = ANY($1)`}},
	{"ใบเบิกของ (Requisition)", []string{`SELECT project_code FROM requisition WHERE project_code = ANY($1)`}},
	{"ใบยืม (Borrow)", []string{`SELECT project_code FROM borrow WHERE project_code = ANY($1)`}},
	{"เงินสดย่อย (Petty Cash)", []string{
		`SELECT project_code FROM petty_cash_requisition WHERE project_code = ANY($1)`,
		`SELECT project_code FROM petty_cash_requisition_line WHERE project_code = ANY($1)`}},
	{"ตัดเบิก/โอนโครงการ (IC)", []string{
		`SELECT project_code FROM ic_project_movement WHERE project_code = ANY($1)`,
		`SELECT to_project_code FROM ic_project_movement_line WHERE to_project_code = ANY($1)`}},
	{"วัสดุโครงการ (IC)", []string{
		`SELECT project_code FROM ic_project_receipt_pool WHERE project_code = ANY($1)`,
		`SELECT project_code FROM ic_project_cost_item WHERE project_code = ANY($1)`}},
	{"สต็อกโครงการ (Project Stock)", []string{`SELECT project_code FROM project_stock WHERE project_code = ANY($1)`}},
	{"ใบโอนสต็อก (Stock Transfer)", []string{
		`SELECT from_project_code FROM stock_transfer WHERE from_project_code = ANY($1)`,
		`SELECT to_project_code FROM stock_transfer WHERE to_project_code = ANY($1)`}},
}

// projectUsageSQL is built once: one UNION ALL query tagged with the source index, so a whole
// page of projects costs a single round trip (no N+1). Single project = one-element array.
var projectUsageSQL = func() string {
	sql := ""
	for i, src := range projectUsageSources {
		for _, sel := range src.selects {
			if sql != "" {
				sql += " UNION "
			}
			sql += "SELECT DISTINCT t.code, " + strconv.Itoa(i) + " AS k FROM (" + sel + ") t(code)"
		}
	}
	return sql
}()

// projectQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, so the same helper runs
// inside the Update transaction.
type projectQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// projectUsage returns, per project_code, the Thai names of the document types that already
// reference it (display order). Codes with no usage are absent from the map.
func projectUsage(ctx context.Context, q projectQuerier, projectCodes []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(projectCodes) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, projectUsageSQL, projectCodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hit := map[string][]bool{}
	for rows.Next() {
		var code string
		var k int
		if err := rows.Scan(&code, &k); err != nil {
			return nil, err
		}
		if hit[code] == nil {
			hit[code] = make([]bool, len(projectUsageSources))
		}
		hit[code][k] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for code, flags := range hit {
		for i, on := range flags {
			if on {
				out[code] = append(out[code], projectUsageSources[i].label)
			}
		}
	}
	return out, nil
}

// fillProjectUsage sets InUse / InUseBy on every project with one query. On failure it logs
// and leaves in_use=false/[] — callers on the read path must not fail because of it; the
// write-side owner guard calls projectUsage directly and fails closed.
func (h *ProjectHandler) fillProjectUsage(ctx context.Context, items []*models.ProjectFull) {
	codes := make([]string, 0, len(items))
	for _, p := range items {
		p.InUseBy = []string{}
		codes = append(codes, p.ProjectCode)
	}
	usage, err := projectUsage(ctx, h.db, codes)
	if err != nil {
		log.Printf("fillProjectUsage: %v", err)
		return
	}
	for _, p := range items {
		if by := usage[p.ProjectCode]; len(by) > 0 {
			p.InUse, p.InUseBy = true, by
		}
	}
}
