package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
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

// parseImportJobCodes parses the free-text "job Code" cell: split on ',', trim, drop empty
// values, upper-case, de-duplicate keeping order. There is NO validation against JobTypes — whatever
// the user types is stored. An empty cell gives an empty (nil) list.
func parseImportJobCodes(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

var importDateTextRe = regexp.MustCompile(`^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$`)

// importDateOK builds YYYY-MM-DD from y/m/d. Only real calendar days with 1900 <= year <= 2400 pass
// (2400 is the cut-off that rejects Buddhist years such as 2569).
func importDateOK(y, m, d int) (string, bool) {
	if y < 1900 || y > 2400 {
		return "", false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return "", false
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true
}

// parseImportDateText accepts ONLY year-first text: "2026-01-01" / "2026/01/01" (one-digit month/day
// tolerated, surrounding spaces/NBSP trimmed). Ambiguous forms (01/02/2026, 1-2-2026) are rejected.
func parseImportDateText(s string) (string, bool) {
	m := importDateTextRe.FindStringSubmatch(normImportSpaces(s))
	if m == nil {
		return "", false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return importDateOK(y, mo, d)
}

// parseImportDateSerial converts an Excel date serial (1900 or 1904 system) to YYYY-MM-DD using the
// calendar fields of the serial itself — the time-of-day fraction is dropped and no time zone is
// applied, so the date can never move a day.
func parseImportDateSerial(serial float64, date1904 bool) (string, bool) {
	if math.IsNaN(serial) || serial < 1 || serial > 2958465 {
		return "", false
	}
	t, err := excelize.ExcelDateToTime(math.Floor(serial), date1904)
	if err != nil {
		return "", false
	}
	return importDateOK(t.Year(), int(t.Month()), t.Day())
}

// normImportSpaces trims leading/trailing whitespace including NBSP and zero-width characters.
func normImportSpaces(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == 0x200B || r == 0xFEFF })
}

// importDateCell reads one start/end date cell straight from the workbook (NOT through GetRows,
// whose display-formatted text turns a real date into "01-02-26"). present=false for an empty cell;
// a numeric cell is an Excel date serial, anything else is text. ok=false when present but invalid.
func importDateCell(xlsx *excelize.File, sheet string, colIdx, rowNo int, date1904 bool) (iso string, present, ok bool) {
	name, err := excelize.CoordinatesToCellName(colIdx+1, rowNo)
	if err != nil {
		return "", false, true
	}
	raw, _ := xlsx.GetCellValue(sheet, name, excelize.Options{RawCellValue: true})
	raw = normImportSpaces(raw)
	if raw == "" {
		return "", false, true
	}
	if t, _ := xlsx.GetCellType(sheet, name); t != excelize.CellTypeSharedString && t != excelize.CellTypeInlineString {
		if f, perr := strconv.ParseFloat(raw, 64); perr == nil {
			iso, ok = parseImportDateSerial(f, date1904)
			return iso, true, ok
		}
	}
	iso, ok = parseImportDateText(raw)
	return iso, true, ok
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
// @Description  Sheet 1, row 1 = header; columns are found by HEADER TEXT (case/space/underscore-insensitive), so the order does not matter and unknown/legacy columns are ignored: project_name* , customer_code* , "Project location_code", start_date, end_date, budget_amount, "job Code". A project_code / consultant_name / consultant_phone / responsible_person_name column in an old file is ignored and never written — project_code is always generated exactly like POST /master/projects ({year}-{customer_code}-{NNN} from project_code_year_counter, Asia/Bangkok year). TWO PASSES, ALL-OR-NOTHING: pass 1 validates every row without writing or allocating any number (customer_code is normalised — spaces/NBSP removed, case-insensitive — and resolved with ONE query; it must exist and be active; the project code uses the customer's STORED code; job Code is FREE TEXT with no validation: comma-separated, each value trimmed/upper-cased, empty values dropped, duplicates removed (order kept), stored as job_codes ("xx, mp ,ME" -> {XX,MP,ME}); an empty cell gives empty job_codes; project_name required; start_date/end_date: real Excel date cells (1900 or 1904 system, no day shift) or year-first text 2026-01-01 / 2026/01/01 (one-digit month/day ok, spaces/NBSP trimmed); ambiguous forms (01/02/2026), years < 1900 or > 2400 are rejected with "แถว N: วันที่เริ่มต้น|วันที่สิ้นสุดไม่ถูกต้อง (ใช้รูปแบบ 2026-01-01 หรือ 2026/01/01)"; empty = no date; there is no end >= start rule; budget_amount numeric >= 0). Any error -> HTTP 400, nothing created, counter untouched, body {success:false, error:"<first messages>", data:{imported:0, failed:N, errors:[{row, reason:"แถว N: …"}]}} listing ALL row errors. Pass 2 runs one transaction that allocates the numbers and inserts every row (rollback gives the numbers back). job_codes = the parsed list; job_shares are never written. Success: 200 {success:true, count:N, data:{imported, rows:[{row,id,project_code}], errors:[]}}.
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
	sheetName := xlsx.GetSheetName(0)
	date1904 := false
	if wp, werr := xlsx.GetWorkbookProps(); werr == nil && wp.Date1904 != nil {
		date1904 = *wp.Date1904
	}
	sheetRows, err := xlsx.GetRows(sheetName)
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
		}{{"start_date", "วันที่เริ่มต้น", &r.startDate}, {"end_date", "วันที่สิ้นสุด", &r.endDate}} {
			idx, has := cols[d.key]
			if !has {
				continue
			}
			iso, present, ok := importDateCell(xlsx, sheetName, idx, rowNo, date1904)
			switch {
			case !present:
			case ok:
				*d.dst = &iso
			default:
				addErr(rowNo, "%sไม่ถูกต้อง (ใช้รูปแบบ 2026-01-01 หรือ 2026/01/01)", d.label)
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
