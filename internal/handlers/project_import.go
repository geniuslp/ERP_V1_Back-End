package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/xuri/excelize/v2"
)

// projectImportHeaderKeys maps a NORMALISED header text (see normImportHeader) to the field it
// feeds. Columns are located by header text, never by position, so an old file that still has
// project_code / consultant_* / responsible_person_name columns keeps working: those headers are
// simply not in this map and are ignored (project_code is never read — the code is always generated).
var projectImportHeaderKeys = map[string]string{
	"projectname":         "project_name",
	"customercode":        "customer_code",
	"projectlocationcode": "location_code",
	"locationcode":        "location_code",
	"startdate":           "start_date",
	"enddate":             "end_date",
	"budgetamount":        "budget_amount",
	"jobcode":             "job_code",
	"jobcodes":            "job_code",
}

// normImportHeader lower-cases a header and drops spaces, '_', '-', '*' so "job Code",
// "job_code" and "Job Code*" all compare equal.
func normImportHeader(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if unicode.IsSpace(r) || r == '_' || r == '-' || r == '*' || r == 0x200B || r == 0xFEFF {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normImportCustomerCode removes every kind of space (incl. NBSP / zero-width) and upper-cases,
// so " wdt " matches the stored "WDT". The stored customer_code is what builds the project code.
func normImportCustomerCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == 0x200B || r == 0xFEFF {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// parseImportJobCodes splits the "job Code" cell on , / ; and whitespace, upper-cases, trims and
// de-duplicates (first occurrence kept, order kept).
func parseImportJobCodes(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == 0x3001 || unicode.IsSpace(r)
	})
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

var projectImportDateLayouts = []string{"2006-01-02", "2006/01/02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006"}

// parseImportDate returns the date as YYYY-MM-DD; ok=false when it is none of the accepted layouts.
func parseImportDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, l := range projectImportDateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

type projectImportRow struct {
	rowNo        int // sheet row (header = 1)
	name         string
	locationCode *string
	startDate    *string
	endDate      *string
	budget       float64
	jobCodes     []string
	customerRaw  string // as typed (trimmed) — for messages
	customerKey  string // normalised lookup key
	customerID   int64
	customerCode string // STORED customer_code, used to build project_code
}

type projectImportError struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

// Import godoc
// @Summary      Bulk import projects from Excel
// @Description  Sheet 1, row 1 = header; columns are found by HEADER TEXT (case/space/underscore-insensitive), so the order does not matter and unknown/legacy columns are ignored: project_name* , customer_code* , "Project location_code", start_date, end_date, budget_amount, "job Code". A project_code / consultant_name / consultant_phone / responsible_person_name column in an old file is ignored and never written — project_code is always generated exactly like POST /master/projects ({year}-{customer_code}-{NNN} from project_code_year_counter, Asia/Bangkok year). TWO PASSES, ALL-OR-NOTHING: pass 1 validates every row without writing or allocating any number (customer_code is normalised — spaces/NBSP removed, case-insensitive — and resolved with ONE query; it must exist and be active; the project code uses the customer's STORED code; job Code accepts "MP,ME" with spaces/lower case/duplicates tolerated and each code must be in JobTypes; project_name required; dates YYYY-MM-DD or DD/MM/YYYY; budget_amount numeric >= 0). Any error -> HTTP 400, nothing created, counter untouched, body {success:false, error:"<first messages>", data:{imported:0, failed:N, errors:[{row, reason:"แถว N: …"}]}} listing ALL row errors. Pass 2 runs one transaction that allocates the numbers and inserts every row (rollback gives the numbers back). job_codes = the normalised list; job_shares are not written. Success: 200 {success:true, count:N, data:{imported, rows:[{row,id,project_code}], errors:[]}}.
// @Tags         Master
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "Excel file (.xlsx)"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/projects/import [post]
func (h *ProjectHandler) Import(c *fiber.Ctx) error {
	const failMsg = "ไม่สามารถนำเข้าโครงการได้ กรุณาลองใหม่อีกครั้ง"
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกไฟล์ Excel")
	}
	f, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถเปิดไฟล์ได้")
	}
	defer f.Close()
	xlsx, err := excelize.OpenReader(f)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ไฟล์ Excel ไม่ถูกต้อง")
	}
	sheetRows, err := xlsx.GetRows(xlsx.GetSheetName(0))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถอ่านชีตแรกของไฟล์ได้")
	}
	if len(sheetRows) < 2 {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลในไฟล์")
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	// ── header mapping (by text) ────────────────────────────────────────────
	cols := map[string]int{}
	for i, hdr := range sheetRows[0] {
		if key, ok := projectImportHeaderKeys[normImportHeader(hdr)]; ok {
			if _, dup := cols[key]; !dup {
				cols[key] = i
			}
		}
	}
	var missing []string
	for _, k := range []string{"project_name", "customer_code"} {
		if _, ok := cols[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบคอลัมน์ในไฟล์: "+strings.Join(missing, ", "))
	}
	cell := func(row []string, key string) string {
		if idx, ok := cols[key]; ok {
			return strings.TrimSpace(cellAt(row, idx))
		}
		return ""
	}

	// ── pass 1a: parse + per-row validation (no DB) ─────────────────────────
	var errs []projectImportError
	addErr := func(rowNo int, format string, a ...any) {
		errs = append(errs, projectImportError{Row: rowNo, Reason: fmt.Sprintf("แถว %d: ", rowNo) + fmt.Sprintf(format, a...)})
	}
	var rows []*projectImportRow
	rowsInFile := 0
	for i, raw := range sheetRows[1:] {
		rowNo := i + 2
		blank := true
		for key := range cols {
			if cell(raw, key) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		rowsInFile++
		r := &projectImportRow{rowNo: rowNo, name: cell(raw, "project_name"), customerRaw: cell(raw, "customer_code")}
		r.customerKey = normImportCustomerCode(r.customerRaw)
		if r.name == "" {
			addErr(rowNo, "ต้องระบุชื่อโครงการ")
		}
		r.locationCode = nullableCell(cell(raw, "location_code"))
		for _, d := range []struct {
			key, label string
			dst        **string
		}{{"start_date", "วันที่เริ่ม", &r.startDate}, {"end_date", "วันที่สิ้นสุด", &r.endDate}} {
			if v := cell(raw, d.key); v != "" {
				if iso, ok := parseImportDate(v); ok {
					*d.dst = &iso
				} else {
					addErr(rowNo, "%s ไม่ถูกต้อง (%s) ใช้รูปแบบ YYYY-MM-DD", d.label, v)
				}
			}
		}
		if v := cell(raw, "budget_amount"); v != "" {
			n, perr := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
			if perr != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				addErr(rowNo, "มูลค่าโครงการไม่ถูกต้อง (%s)", v)
			} else {
				r.budget = n
			}
		}
		r.jobCodes = parseImportJobCodes(cell(raw, "job_code"))
		for _, jc := range r.jobCodes {
			if !validJobCodes[jc] {
				addErr(rowNo, "ไม่พบประเภทงาน %s", jc)
			}
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลในไฟล์")
	}

	// ── pass 1b: customers, ONE query ───────────────────────────────────────
	keys := make([]string, 0, len(rows))
	seenKey := map[string]bool{}
	for _, r := range rows {
		if r.customerKey != "" && !seenKey[r.customerKey] {
			seenKey[r.customerKey] = true
			keys = append(keys, r.customerKey)
		}
	}
	type custInfo struct {
		id     int64
		code   string
		active bool
	}
	customers := map[string]custInfo{}
	if len(keys) > 0 {
		crows, qerr := h.db.Query(ctx, `
			SELECT cus_id, btrim(customer_code), is_active, upper(btrim(customer_code))
			FROM customer WHERE upper(btrim(customer_code)) = ANY($1)`, keys)
		if qerr != nil {
			log.Printf("project import: customer lookup: %v", qerr)
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		for crows.Next() {
			var ci custInfo
			var key string
			if serr := crows.Scan(&ci.id, &ci.code, &ci.active, &key); serr != nil {
				crows.Close()
				log.Printf("project import: customer scan: %v", serr)
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
			customers[key] = ci
		}
		crows.Close()
		if crows.Err() != nil {
			log.Printf("project import: customer rows: %v", crows.Err())
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
	}
	for _, r := range rows {
		ci, ok := customers[r.customerKey]
		switch {
		case r.customerKey == "":
			addErr(r.rowNo, "ไม่พบรหัสลูกค้า (ไม่ได้ระบุ)")
		case !ok || ci.code == "":
			addErr(r.rowNo, "ไม่พบรหัสลูกค้า %s", r.customerRaw)
		case !ci.active:
			addErr(r.rowNo, "รหัสลูกค้า %s ถูกปิดใช้งาน", r.customerRaw)
		default:
			r.customerID, r.customerCode = ci.id, ci.code
		}
	}

	// ── pass 1 verdict ──────────────────────────────────────────────────────
	if len(errs) > 0 {
		sort.SliceStable(errs, func(a, b int) bool { return errs[a].Row < errs[b].Row })
		msgs := make([]string, 0, 10)
		for i, e := range errs {
			if i == 10 {
				msgs = append(msgs, fmt.Sprintf("และอีก %d รายการ", len(errs)-10))
				break
			}
			msgs = append(msgs, e.Reason)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("นำเข้าไม่สำเร็จ พบข้อผิดพลาด %d รายการ (ยังไม่มีการสร้างโครงการ): %s", len(errs), strings.Join(msgs, "; ")),
			"data": fiber.Map{
				"rows_in_file": rowsInFile, "imported": 0, "failed": len(errs), "rows": []any{}, "errors": errs,
			},
		})
	}

	// ── pass 2: one transaction, numbers + inserts ──────────────────────────
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	defer tx.Rollback(ctx)
	year := bangkokNow().Year()
	type okRow struct {
		Row         int    `json:"row"`
		ID          int64  `json:"id"`
		ProjectCode string `json:"project_code"`
	}
	oks := make([]okRow, 0, len(rows))
	for _, r := range rows {
		var id int64
		var code string
		for attempt := 0; attempt < 5 && id == 0; attempt++ {
			if code, err = nextProjectCode(ctx, tx, year, r.customerCode); err != nil {
				log.Printf("project import: nextProjectCode: %v", err)
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
			sp, serr := tx.Begin(ctx) // savepoint: a taken code is retried with the next number
			if serr != nil {
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
			ierr := sp.QueryRow(ctx, `
				INSERT INTO project
				    (project_code, project_name, location_code, customer_id, job_codes, budget_amount,
				     start_date, end_date, status, is_active, created_at, updated_at, created_by, updated_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',true,now(),now(),$9,$9)
				RETURNING id`,
				code, r.name, r.locationCode, r.customerID, r.jobCodes, r.budget,
				r.startDate, r.endDate, claims.UserID).Scan(&id)
			if ierr == nil {
				if cerr := sp.Commit(ctx); cerr != nil {
					return fiber.NewError(fiber.StatusInternalServerError, failMsg)
				}
				break
			}
			_ = sp.Rollback(ctx)
			if pgErr, ok := ierr.(*pgconn.PgError); ok && pgErr.Code == "23505" {
				id = 0
				continue
			}
			log.Printf("project import: insert row %d: %v", r.rowNo, ierr)
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		if id == 0 {
			return fiber.NewError(fiber.StatusConflict, "ไม่สามารถสร้างรหัสโครงการที่ไม่ซ้ำได้ กรุณาลองใหม่อีกครั้ง")
		}
		oks = append(oks, okRow{Row: r.rowNo, ID: id, ProjectCode: code})
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("project import: commit: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	return c.JSON(fiber.Map{
		"success": true,
		"count":   len(oks),
		"data": fiber.Map{
			"rows_in_file": rowsInFile, "imported": len(oks), "failed": 0, "rows": oks, "errors": []projectImportError{},
		},
	})
}
