package handlers

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// codeNamePair is one row of a code/name lookup list (dept_code+dept_name, job_code+label, …)
// shared between Excel template generation and import-time validation.
type codeNamePair struct {
	Code string
	Name string
}

// fetchActiveDepartments returns dept_code/dept_name for every active department, used both
// to populate the template's Lookup sheet and to validate dept_code at import time.
func fetchActiveDepartments(ctx context.Context, db *pgxpool.Pool) ([]codeNamePair, error) {
	rows, err := db.Query(ctx, `SELECT dept_code, dept_name FROM departments WHERE is_active = true ORDER BY dept_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []codeNamePair
	for rows.Next() {
		var p codeNamePair
		if err := rows.Scan(&p.Code, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// projectJobTypeOptions returns the fixed JobTypes enum (job_type.go) as code/name pairs —
// this is what project.job_codes[] is actually validated against (ValidateJobCode), NOT the
// unrelated cost_job master table, despite both being called "job_code" in this codebase.
func projectJobTypeOptions() []codeNamePair {
	out := make([]codeNamePair, 0, len(JobTypes))
	for _, jt := range JobTypes {
		out = append(out, codeNamePair{Code: jt.Code, Name: jt.Label})
	}
	return out
}

// creditTermOptions is the fixed, hardcoded credit_term dropdown list shared by the Supplier
// page's credit_term dropdown, and now also the Customer/Project Excel templates and
// import-time validation. Sourced from the actual frontend list (user-confirmed, 2026-09-08) —
// keep in sync with erp-frontend's Supplier credit_term dropdown by hand, the same way
// job_type.go's JobTypes is kept in sync with erp-frontend/src/constants/jobTypes.ts.
var creditTermOptions = []string{"เงินสด", "15 วัน", "30 วัน", "45 วัน", "60 วัน", "90 วัน"}

// creditTermOptionsExcludingCash is creditTermOptions with "เงินสด" removed — used only for
// the Customer-side lookup/dropdown, per the rule that a customer's credit_term is never "cash
// on delivery" the way a supplier payment term can be. Project and Supplier keep the full list.
func creditTermOptionsExcludingCash() []string {
	out := make([]string, 0, len(creditTermOptions)-1)
	for _, v := range creditTermOptions {
		if v != "เงินสด" {
			out = append(out, v)
		}
	}
	return out
}

// toSet builds a membership set out of a code/name pair list, for O(1) import-time validation.
func toSet(pairs []codeNamePair) map[string]bool {
	m := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		m[p.Code] = true
	}
	return m
}

func toStringSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// writeLookupSheet creates a hidden "Lookup" sheet holding the current valid dept_code/
// job_code/credit_term option lists, and returns the Excel range references (e.g.
// "Lookup!$A$2:$A$11") to bind as dropdown sources via DataValidation.SetSqrefDropList.
// Shared by both the Project and Customer template generators (Customer only uses the
// credit range; the others are harmless if left unreferenced).
func writeLookupSheet(f *excelize.File, depts, jobs []codeNamePair, credits []string) (deptRange, jobRange, creditRange string, err error) {
	const sheet = "Lookup"
	if _, err = f.NewSheet(sheet); err != nil {
		return "", "", "", err
	}

	f.SetCellValue(sheet, "A1", "dept_code")
	f.SetCellValue(sheet, "B1", "dept_name")
	f.SetCellValue(sheet, "C1", "job_code")
	f.SetCellValue(sheet, "D1", "job_name")
	f.SetCellValue(sheet, "E1", "credit_term")

	maxRows := len(depts)
	if len(jobs) > maxRows {
		maxRows = len(jobs)
	}
	if len(credits) > maxRows {
		maxRows = len(credits)
	}
	if maxRows == 0 {
		maxRows = 1
	}

	for i := 0; i < len(depts); i++ {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), depts[i].Code)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), depts[i].Name)
	}
	for i := 0; i < len(jobs); i++ {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), jobs[i].Code)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), jobs[i].Name)
	}
	for i := 0; i < len(credits); i++ {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), credits[i])
	}

	lastRow := maxRows + 1
	deptRange = fmt.Sprintf("Lookup!$A$2:$A$%d", lastRow)
	jobRange = fmt.Sprintf("Lookup!$C$2:$C$%d", lastRow)
	creditRange = fmt.Sprintf("Lookup!$E$2:$E$%d", lastRow)

	if err = f.SetSheetVisible(sheet, false); err != nil {
		return "", "", "", err
	}
	return deptRange, jobRange, creditRange, nil
}

// colLetter converts a 0-based column index to an Excel column letter (0 -> "A", 25 -> "Z",
// 26 -> "AA", …), safe beyond the single-letter range unlike raw rune('A'+i) arithmetic.
func colLetter(idx int) string {
	name, _ := excelize.ColumnNumberToName(idx + 1)
	return name
}

// addDropdown attaches a list-type data validation to sqref (e.g. "H2:H1000"), sourced from
// a Lookup-sheet range (e.g. "Lookup!$A$2:$A$11"). Errors on out-of-range cells rather than
// silently accepting free text.
func addDropdown(f *excelize.File, sheet, sqref, sourceRange string) error {
	dv := excelize.NewDataValidation(true)
	dv.SetSqref(sqref)
	dv.SetSqrefDropList(sourceRange)
	dv.SetError(excelize.DataValidationErrorStyleStop, "ค่าไม่ถูกต้อง", "กรุณาเลือกค่าจากรายการที่กำหนด")
	return f.AddDataValidation(sheet, dv)
}
