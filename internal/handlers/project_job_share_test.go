package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"erp-api/internal/models"
)

func TestJobNeedsShare(t *testing.T) {
	for _, code := range []string{"FS", "FP", "FB", "G", "OH", "HO", "MP", "MH", "DE", "RE"} {
		t.Logf("JobNeedsShare(%s) = %v", code, JobNeedsShare(code))
	}
	want := map[string]bool{"FS": false, "FP": false, "FB": false, "G": false, "OH": false, "HO": true, "MH": true, "DE": true, "RE": true, "MP": true}
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
	codes := []string{"MP", "ME", "MS", "FS", "G"}
	cases := []struct {
		name     string
		shares   []models.ProjectJobShareInput
		wantErr  string   // substring, "" = ok
		wantRows []string // "CODE=pct" rows expected back (only when ok)
	}{
		{"ok", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "30")}, "", []string{"MP=40.00", "ME=30.00", "MS=30.00"}},
		{"no shares at all (needs-share jobs, none given)", nil, "", []string{}},
		{"missing MS is allowed", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30")}, "", []string{"MP=40.00", "ME=30.00"}},
		{"total 120 not rejected by the app", []models.ProjectJobShareInput{sh("MP", "60"), sh("ME", "30"), sh("MS", "30")}, "", []string{"MP=60.00", "ME=30.00", "MS=30.00"}},
		{"zero is skipped", []models.ProjectJobShareInput{sh("MP", "0"), sh("ME", "30"), sh("MS", "30")}, "", []string{"ME=30.00", "MS=30.00"}},
		{"negative is skipped", []models.ProjectJobShareInput{sh("MP", "-5"), sh("ME", "30")}, "", []string{"ME=30.00"}},
		{"empty percent is skipped", []models.ProjectJobShareInput{sh("MP", ""), sh("ME", "30")}, "", []string{"ME=30.00"}},
		{"0.1+0.2 style exact", []models.ProjectJobShareInput{sh("MP", "33.33"), sh("ME", "33.33"), sh("MS", "33.34")}, "", []string{"MP=33.33", "ME=33.33", "MS=33.34"}},
		// rules that are kept
		{"share on FS", []models.ProjectJobShareInput{sh("MP", "40"), sh("FS", "10")}, "ไม่ต้องกำหนดสัดส่วน", nil},
		{"share on G", []models.ProjectJobShareInput{sh("MP", "40"), sh("G", "10")}, "ไม่ต้องกำหนดสัดส่วน", nil},
		{"share on OH not in project", []models.ProjectJobShareInput{sh("MP", "40"), sh("OH", "10")}, "ไม่ได้อยู่ในรายการงาน", nil},
		{"duplicate", []models.ProjectJobShareInput{sh("MP", "10"), sh("MP", "10")}, "ซ้ำกัน", nil},
		{"3 decimals", []models.ProjectJobShareInput{sh("MP", "33.333"), sh("ME", "30")}, "ทศนิยมไม่เกิน 2", nil},
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
	for in, want := range map[string]string{"2026-01-31": "2026-01-31", "31/01/2026": "2026-01-31", "1/2/2026": "2026-02-01"} {
		if got, ok := parseImportDate(in); !ok || got != want {
			t.Errorf("date %q = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := parseImportDate("31/13/2026"); ok {
		t.Error("invalid date accepted")
	}
	for _, h := range []string{"job Code", "job_code", " Job Code* "} {
		if projectImportHeaderKeys[normImportHeader(h)] != "job_code" {
			t.Errorf("header %q not mapped", h)
		}
	}
}
