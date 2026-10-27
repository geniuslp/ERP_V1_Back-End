package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"erp-api/internal/models"
)

func TestJobNeedsShare(t *testing.T) {
	for _, code := range []string{"FS", "FP", "FB", "G", "OH", "HO", "MS", "AC", "DE", "RE"} {
		t.Logf("JobNeedsShare(%s) = %v", code, JobNeedsShare(code))
	}
	want := map[string]bool{"FS": false, "FP": false, "FB": false, "G": false, "OH": false, "HO": true, "AC": true, "FF": true, "DE": true, "RE": true, "MS": true}
	for code, w := range want {
		if got := JobNeedsShare(code); got != w {
			t.Errorf("JobNeedsShare(%q) = %v, want %v", code, got, w)
		}
	}
}

func sh(code, pct string) models.ProjectJobShareInput {
	return models.ProjectJobShareInput{JobCode: code, SharePercent: json.Number(pct)}
}

func TestValidateJobShares(t *testing.T) {
	codes := []string{"MS", "EE", "SN", "FS", "G"}
	cases := []struct {
		name     string
		shares   []models.ProjectJobShareInput
		wantErr  string   // substring, "" = ok
		wantRows []string // "CODE=pct" rows expected back (only when ok)
	}{
		{"ok", []models.ProjectJobShareInput{sh("MS", "40"), sh("EE", "30"), sh("SN", "30")}, "", []string{"MS=40.00", "EE=30.00", "SN=30.00"}},
		{"no shares at all (needs-share jobs, none given)", nil, "", []string{}},
		{"missing MS is allowed", []models.ProjectJobShareInput{sh("MS", "40"), sh("EE", "30")}, "", []string{"MS=40.00", "EE=30.00"}},
		{"total 120 not rejected by the app", []models.ProjectJobShareInput{sh("MS", "60"), sh("EE", "30"), sh("SN", "30")}, "", []string{"MS=60.00", "EE=30.00", "SN=30.00"}},
		{"zero is skipped", []models.ProjectJobShareInput{sh("MS", "0"), sh("EE", "30"), sh("SN", "30")}, "", []string{"EE=30.00", "SN=30.00"}},
		{"negative is skipped", []models.ProjectJobShareInput{sh("MS", "-5"), sh("EE", "30")}, "", []string{"EE=30.00"}},
		{"empty percent is skipped", []models.ProjectJobShareInput{sh("MS", ""), sh("EE", "30")}, "", []string{"EE=30.00"}},
		{"0.1+0.2 style exact", []models.ProjectJobShareInput{sh("MS", "33.33"), sh("EE", "33.33"), sh("SN", "33.34")}, "", []string{"MS=33.33", "EE=33.33", "SN=33.34"}},
		// rules that are kept
		{"share on FS", []models.ProjectJobShareInput{sh("MS", "40"), sh("FS", "10")}, "ไม่ต้องกำหนดสัดส่วน", nil},
		{"share on G", []models.ProjectJobShareInput{sh("MS", "40"), sh("G", "10")}, "ไม่ต้องกำหนดสัดส่วน", nil},
		{"share on OH not in project", []models.ProjectJobShareInput{sh("MS", "40"), sh("OH", "10")}, "ไม่ได้อยู่ในรายการงาน", nil},
		{"duplicate", []models.ProjectJobShareInput{sh("MS", "10"), sh("MS", "10")}, "ซ้ำกัน", nil},
		{"3 decimals", []models.ProjectJobShareInput{sh("MS", "33.333"), sh("EE", "30")}, "ทศนิยมไม่เกิน 2", nil},
	}
	for _, c := range cases {
		got, err := validateJobShares(codes, c.shares)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error %v, want containing %q", c.name, err, c.wantErr)
			continue
		}
		if c.wantErr == "" {
			rows := []string{}
			for _, g := range got {
				rows = append(rows, g.jobCode+"="+g.percent)
			}
			if strings.Join(rows, ",") != strings.Join(c.wantRows, ",") {
				t.Errorf("%s: rows %v, want %v", c.name, rows, c.wantRows)
			}
		}
	}
}

func TestRecodeProjectCode(t *testing.T) {
	cases := []struct {
		cur, cust, want string
		ok              bool
	}{
		{"2026-WDT-006", "ABC", "2026-ABC-006", true},
		{"2026-WDT-006", " ABC ", "2026-ABC-006", true},
		{"2026-A-B-006", "XY-Z", "2026-XY-Z-006", true}, // customer_code may contain '-'
		{"2026-WDT-1234", "ABC", "2026-ABC-1234", true}, // 4+ digit number kept as is
		{"PRJ-000001", "ABC", "", false},                // legacy code
		{"2026-WDT-006", "", "", false},                 // customer without code
		{"2026-WDT-", "ABC", "", false},
	}
	for _, c := range cases {
		got, ok := recodeProjectCode(c.cur, c.cust)
		if ok != c.ok || got != c.want {
			t.Errorf("recodeProjectCode(%q,%q) = %q,%v want %q,%v", c.cur, c.cust, got, ok, c.want, c.ok)
		}
	}
}

func TestProjectImportParsers(t *testing.T) {
	if got := strings.Join(parseImportJobCodes("xx, mp ,ME,,mp"), "|"); got != "XX|MP|ME" {
		t.Errorf("job codes = %q", got)
	}
	if got := normImportCustomerCode(" w dt ​"); got != "WDT" {
		t.Errorf("customer code = %q", got)
	}
	for in, want := range map[string]string{"2026-01-31": "2026-01-31", "2026/1/2": "2026-01-02", " 2026/01/01 ": "2026-01-01"} {
		if got, ok := parseImportDateText(in); !ok || got != want {
			t.Errorf("date %q = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"01/02/2026", "1-2-2026", "2569-01-01", "1899-12-31", "2026-02-30", "2026-13-01", "20260101", "2026-01-01 10:00"} {
		if got, ok := parseImportDateText(in); ok {
			t.Errorf("date %q accepted as %q", in, got)
		}
	}
	if got, ok := parseImportDateSerial(46023, false); !ok || got != "2026-01-01" {
		t.Errorf("serial 46023 = %q,%v", got, ok)
	}
	if got, ok := parseImportDateSerial(46023.99, false); !ok || got != "2026-01-01" {
		t.Errorf("serial 46023.99 = %q,%v", got, ok)
	}
	if got, ok := parseImportDateSerial(46023-1462, true); !ok || got != "2026-01-01" {
		t.Errorf("1904 serial = %q,%v", got, ok)
	}
	for _, h := range []string{"job Code", "job_code", " Job Code* "} {
		if projectImportHeaderKeys[normImportHeader(h)] != "job_code" {
			t.Errorf("header %q not mapped", h)
		}
	}
}
