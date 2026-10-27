package handlers

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// JobNeedsShare is the single source of truth for "does this project job code carry a
// share_percent of the project budget". The stock jobs (FS, FP, FB), "G" and "OH" do not. It is an
// explicit list because "FF" (Fire Protection) also starts with F but does carry a share. Validation
// and the needs_share flag both call this - never re-implement the rule elsewhere.
func JobNeedsShare(code string) bool {
	switch code {
	case "FS", "FP", "FB", "G", "OH":
		return false
	}
	return true
}

// ProjectJobShare is one row of project_job_share as returned by GET /master/projects/:id.
// share_amount is derived (budget_amount * share_percent / 100), never stored.
type ProjectJobShare struct {
	JobCode      string  `json:"job_code"`
	SharePercent float64 `json:"share_percent"`
	ShareAmount  float64 `json:"share_amount"`
}

// ProjectDetail is a project plus its job shares (detail / create / update responses only —
// the list endpoint keeps returning models.ProjectFull unchanged).
type ProjectDetail struct {
	models.ProjectFull
	JobShares []ProjectJobShare `json:"job_shares"`
}

type validatedJobShare struct {
	jobCode string
	percent string // exact decimal text, e.g. "33.30"
}

var ratHundred = big.NewRat(100, 1)

// validateJobShares checks the submitted shares against the submitted job_codes. Pure
// function (no SQL); all sums use exact big.Rat arithmetic, never floats.
//
// Share-vs-value is deliberately NOT enforced here: a needs-share job may come without a
// percent (missing / empty / zero / negative), and the total may exceed 100 — such entries
// are simply not returned (so no project_job_share row is written for them; the table CHECK
// requires share_percent > 0). The job stays in project.job_codes. A total above 100 is
// caught by the DB trigger and mapped to a Thai 400 by jobShareDBError.
func validateJobShares(jobCodes []string, shares []models.ProjectJobShareInput) ([]validatedJobShare, error) {
	bad := func(format string, a ...any) error {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(format, a...))
	}

	inProject := map[string]bool{}
	for _, jc := range jobCodes {
		inProject[jc] = true
	}

	seen := map[string]bool{}
	out := make([]validatedJobShare, 0, len(shares))
	for _, s := range shares {
		code := strings.TrimSpace(s.JobCode)
		if code == "" {
			return nil, bad("ต้องระบุรหัสงานในสัดส่วนทุกรายการ")
		}
		if seen[code] {
			return nil, bad("สัดส่วนของงาน %s ซ้ำกัน", code)
		}
		seen[code] = true
		if !inProject[code] {
			return nil, bad("งาน %s ไม่ได้อยู่ในรายการงานของโครงการ จึงกำหนดสัดส่วนไม่ได้", code)
		}
		if !JobNeedsShare(code) {
			return nil, bad("งาน %s ไม่ต้องกำหนดสัดส่วน", code)
		}
		raw := strings.TrimSpace(s.SharePercent.String())
		if raw == "" {
			continue // no percent given: keep the job, write no share row
		}
		pct, ok := new(big.Rat).SetString(raw)
		if !ok {
			return nil, bad("สัดส่วนของงาน %s ต้องเป็นตัวเลข", code)
		}
		if pct.Sign() <= 0 {
			continue // zero / negative = no share; the table CHECK needs share_percent > 0
		}
		if !new(big.Rat).Mul(pct, ratHundred).IsInt() {
			return nil, bad("สัดส่วนของงาน %s ต้องมีทศนิยมไม่เกิน 2 ตำแหน่ง", code)
		}
		out = append(out, validatedJobShare{jobCode: code, percent: pct.FloatString(2)})
	}
	return out, nil
}

// jobShareDBError maps the DB-level share guards to a plain Thai 400 so the raw Postgres
// error never reaches the client: the deferred trigger trg_project_job_share_total (RAISE
// EXCEPTION, SQLSTATE P0001, raised at COMMIT) and the share_percent CHECK (23514, at INSERT).
// Returns nil for any other error.
func jobShareDBError(err error, shares []validatedJobShare) error {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return nil
	}
	switch {
	case pgErr.Code == "P0001" && strings.Contains(pgErr.Message, "project_job_share total"):
		total := new(big.Rat)
		for _, s := range shares {
			if p, ok := new(big.Rat).SetString(s.percent); ok {
				total.Add(total, p)
			}
		}
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("สัดส่วนรวมต้องไม่เกิน 100%% (ขณะนี้รวม %s%%)", total.FloatString(2)))
	case pgErr.Code == "23514" && pgErr.ConstraintName == "project_job_share_share_percent_check":
		return fiber.NewError(fiber.StatusBadRequest, "สัดส่วนของแต่ละงานต้องไม่เกิน 100%")
	}
	return nil
}

// replaceJobShares replaces the project's project_job_share rows inside the caller's
// transaction (DELETE then INSERT).
func replaceJobShares(ctx context.Context, tx pgx.Tx, projectID int64, shares []validatedJobShare) error {
	if _, err := tx.Exec(ctx, `DELETE FROM project_job_share WHERE project_id = $1`, projectID); err != nil {
		return err
	}
	for _, s := range shares {
		if _, err := tx.Exec(ctx,
			`INSERT INTO project_job_share (project_id, job_code, share_percent) VALUES ($1, $2, $3::text::numeric)`,
			projectID, s.jobCode, s.percent); err != nil {
			return err
		}
	}
	return nil
}

// loadJobShares returns the project's shares with the derived share_amount; non-nil on success.
// A query/scan failure is logged with the real error and surfaced as a plain 500 (never as an
// empty list, which would look like a legacy project).
func (h *ProjectHandler) loadJobShares(ctx context.Context, projectID int64) ([]ProjectJobShare, error) {
	out := []ProjectJobShare{}
	fail := func(err error) ([]ProjectJobShare, error) {
		log.Printf("loadJobShares project_id=%d: %v", projectID, err)
		return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดสัดส่วนงานได้ กรุณาลองใหม่อีกครั้ง")
	}
	rows, err := h.db.Query(ctx, `
		SELECT s.job_code, s.share_percent::float8,
		       (p.budget_amount * s.share_percent / 100)::float8
		FROM project_job_share s
		JOIN project p ON p.id = s.project_id
		WHERE s.project_id = $1
		ORDER BY COALESCE(array_position(p.job_codes, s.job_code), 999), s.job_code`, projectID)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	for rows.Next() {
		var r ProjectJobShare
		if err := rows.Scan(&r.JobCode, &r.SharePercent, &r.ShareAmount); err != nil {
			return fail(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return out, nil
}

// JobTypeOption is one selectable project job type for GET /master/job-types.
type JobTypeOption struct {
	JobCode    string `json:"job_code"`
	JobName    string `json:"job_name"`
	NeedsShare bool   `json:"needs_share"` // JobNeedsShare(job_code) — frontend must not hardcode the rule
}

// ListJobTypes godoc
// @Summary      List project job types
// @Description  Every job code accepted in project.job_codes (the fixed JobTypes list in job_type.go) with needs_share computed by JobNeedsShare (false for F* codes, G and OH). The frontend must use this flag instead of hardcoding the rule.
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/job-types [get]
func (h *ProjectHandler) ListJobTypes(c *fiber.Ctx) error {
	out := make([]JobTypeOption, 0, len(JobTypes))
	for _, jt := range JobTypes {
		out = append(out, JobTypeOption{JobCode: jt.Code, JobName: jt.Label, NeedsShare: JobNeedsShare(jt.Code)})
	}
	return c.JSON(fiber.Map{"success": true, "data": out})
}
