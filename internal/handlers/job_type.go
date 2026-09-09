package handlers

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobTypeInfo is one entry in the fixed, hardcoded Job Type list shared by PR (job_code),
// PO (job_code), and Project (job_codes[]). This is NOT a master/lookup table — the list is
// fixed at 12 values and stored directly as a varchar column, keyed by short code (e.g. "MP"),
// never by the numeric 001-012 reference prefix and never by the label.
type JobTypeInfo struct {
	RefNo string // "001".."012" — reference/display order only, never stored
	Code  string // stored value, e.g. "MP"
	Label string
}

// JobTypes is the full fixed list. Keep in sync with erp-frontend/src/constants/jobTypes.ts.
var JobTypes = []JobTypeInfo{
	{"001", "MP", "Metal Structure"},
	{"002", "ME", "Electrical system work"},
	{"003", "MS", "Sanitary System"},
	{"004", "MF", "Fire Protection"},
	{"005", "MG", "GAS System"},
	{"006", "MH", "HVAC / BAS / Clean Room-Cold Room"},
	{"007", "FS", "Stock FAC-S"},
	{"008", "FP", "Stock FAC-P"},
	{"009", "FB", "Stock FAC-BO"},
	{"010", "DE", "Dead Stock"},
	{"011", "RE", "Return Project"},
	{"012", "OH", "General Code"},
}

// validJobCodes is the lookup used by ValidateJobCode/validateJobCodes. Built once from
// JobTypes so the 12-value list only ever needs to be edited in one place.
var validJobCodes = func() map[string]bool {
	m := make(map[string]bool, len(JobTypes))
	for _, jt := range JobTypes {
		m[jt.Code] = true
	}
	return m
}()

// ValidateJobCode rejects any code outside the fixed 12-value JobTypes set. Shared by PR, PO,
// and Project (job_codes[]) handlers so the allowed-values list is never duplicated.
func ValidateJobCode(code string) error {
	if !validJobCodes[code] {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("invalid job_code: %q", code))
	}
	return nil
}

// validateJobCodes rejects any job_codes entry outside the fixed JobTypes set — mirrors the
// oneof-style validation a single enum field would get via a struct tag, done manually here
// since Project.JobCodes is a slice.
func validateJobCodes(codes []string) error {
	for _, jc := range codes {
		if err := ValidateJobCode(jc); err != nil {
			return err
		}
	}
	return nil
}

// validateJobCodeForProject checks jobCode is one of the fixed 12 codes, and — when
// projectCode is non-nil/non-blank — additionally that it's a member of that project's
// job_codes[] array. If the project_code doesn't exist, this silently skips the project-based
// check (pgx.ErrNoRows) and leaves the bad-FK case to whatever project_code existence check
// the caller already runs (Create/Update in pr.go); it is not this function's job to duplicate
// that error. Shared by PR Create/Update (job_code is only project-restricted on PR today).
func validateJobCodeForProject(ctx context.Context, db *pgxpool.Pool, jobCode string, projectCode *string) error {
	if err := ValidateJobCode(jobCode); err != nil {
		return err
	}
	if projectCode == nil || *projectCode == "" {
		return nil
	}

	var allowed []string
	err := db.QueryRow(ctx, `SELECT job_codes FROM project WHERE project_code=$1`, *projectCode).Scan(&allowed)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	for _, jc := range allowed {
		if jc == jobCode {
			return nil
		}
	}
	return fiber.NewError(fiber.StatusBadRequest, "ประเภทงานที่เลือกไม่ตรงกับโครงการนี้")
}
