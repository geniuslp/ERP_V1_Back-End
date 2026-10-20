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
