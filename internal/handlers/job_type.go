package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
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
	{"012", "G", "General Code"},
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
