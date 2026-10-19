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
		name    string
		shares  []models.ProjectJobShareInput
		wantErr string // substring, "" = ok
	}{
		{"ok", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "30")}, ""},
		{"total 100.01", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "30.01")}, "ขณะนี้รวม 100.01%"},
		{"missing MS", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30")}, "ของงาน MS"},
		{"share on FS", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "20"), sh("FS", "10")}, "ไม่ต้องกำหนดสัดส่วน"},
		{"share on G", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "20"), sh("G", "10")}, "ไม่ต้องกำหนดสัดส่วน"},
		{"share on OH not in project", []models.ProjectJobShareInput{sh("MP", "40"), sh("ME", "30"), sh("MS", "20"), sh("OH", "10")}, "ไม่ได้อยู่ในรายการงาน"},
		{"duplicate", []models.ProjectJobShareInput{sh("MP", "10"), sh("MP", "10"), sh("ME", "10"), sh("MS", "10")}, "ซ้ำกัน"},
		{"zero", []models.ProjectJobShareInput{sh("MP", "0"), sh("ME", "30"), sh("MS", "30")}, "มากกว่า 0"},
		{"3 decimals", []models.ProjectJobShareInput{sh("MP", "33.333"), sh("ME", "30"), sh("MS", "30")}, "ทศนิยมไม่เกิน 2"},
		{"0.1+0.2 style exact", []models.ProjectJobShareInput{sh("MP", "33.33"), sh("ME", "33.33"), sh("MS", "33.34")}, ""},
	}
	for _, c := range cases {
		_, err := validateJobShares(codes, c.shares)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error %v, want containing %q", c.name, err, c.wantErr)
		}
		if err != nil {
			t.Logf("%s -> %v", c.name, err)
		}
	}
}
