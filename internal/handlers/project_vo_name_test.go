package handlers

import "testing"

func TestCheckVOProjectName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"Q - Extra work A", true},
		{"Q - x", true},
		{"Q - ทดสอบ", true},
		{"Q - ", false},
		{"Q -    ", false},
		{"Q -", false},
		{"Q - \t", false},
		{"Q-Extra", false},
		{"q - Extra", false},
		{" Q - Extra", false},
		{"Extra work", false},
		{"", false},
	}
	for _, c := range cases {
		err := checkVOProjectName(c.name)
		if (err == nil) != c.ok {
			t.Errorf("checkVOProjectName(%q): ok=%v, err=%v", c.name, c.ok, err)
		}
	}
}
