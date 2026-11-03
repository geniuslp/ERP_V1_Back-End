package handlers

import "testing"

// retention_mode of a contractor bill: empty means EVERY, only EVERY / ONCE are accepted.
func TestValidateWOBillRetentionMode(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "EVERY", false},
		{"EVERY", "EVERY", false},
		{"ONCE", "ONCE", false},
		{"MONTHLY", "", true},
	}
	for _, c := range cases {
		req := woBillRequest{BillType: "PROGRESS", RetentionMode: c.in}
		err := validateWOBillRequest(&req)
		if (err != nil) != c.wantErr {
			t.Errorf("mode %q: err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && req.RetentionMode != c.want {
			t.Errorf("mode %q normalised to %q, want %q", c.in, req.RetentionMode, c.want)
		}
	}
}
