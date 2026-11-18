package core

import (
	"testing"
	"time"
)

func TestSchoolYear(t *testing.T) {
	for _, tc := range []struct {
		date string
		want int
	}{
		{"2026-11-15", 2026},
		{"2026-08-01", 2026},
		{"2027-03-10", 2026},
		{"2026-07-31", 2025},
	} {
		d, err := time.Parse(time.DateOnly, tc.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := schoolYear(d); got != tc.want {
			t.Errorf("schoolYear(%s) = %d, want %d", tc.date, got, tc.want)
		}
	}
}
