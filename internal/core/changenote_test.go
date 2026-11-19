package core

import (
	"maps"
	"testing"
)

func TestParseChangeNote(t *testing.T) {
	for _, tc := range []struct {
		note    string
		changes []string
		lang    string
		prev    map[string]string
		rest    string
	}{
		{"Het lokaal is gewijzigd (was 127).", []string{"location"}, "nl", map[string]string{"location": "127"}, ""},
		{"De docent is vervangen (was srs).", []string{"teacher"}, "nl", map[string]string{"teacher": "srs"}, ""},
		{"De docent is gewijzigd (was Mevrouw Dekker).", []string{"teacher"}, "nl", map[string]string{"teacher": "Mevrouw Dekker"}, ""},
		{"Er is een wijziging bij docenten: Kersjes,Knuit wordt Knuit.", []string{"teacher"}, "nl", map[string]string{"teacher": "Kersjes, Knuit"}, ""},
		{"De les is verplaatst (komt van vr 13:15u).", []string{"time"}, "nl", map[string]string{"time": "vr 13:15"}, ""},
		{"De les is verplaatst (komt van vr 13:15u).", []string{"time"}, "en", map[string]string{"time": "Fri 13:15"}, ""},
		{"De activiteit is verplaatst (komt van vr 14:55u).", []string{"time"}, "nl", map[string]string{"time": "vr 14:55"}, ""},
		{"De les is verplaatst (was het 3e uur).", []string{"time"}, "nl", map[string]string{"time": "het 3e uur"}, ""},
		{
			"Het lokaal is gewijzigd (was 108) en er is een wijziging bij docenten: Kersjes,Knuit wordt Kersjes.",
			[]string{"location", "teacher"}, "nl",
			map[string]string{"location": "108", "teacher": "Kersjes, Knuit"}, "",
		},
		{
			"Het lokaal is gewijzigd (was 234) en de les is verplaatst (komt van vr 11:55u).",
			[]string{"location", "time"}, "nl",
			map[string]string{"location": "234", "time": "vr 11:55"}, "",
		},
		{
			"De les is verplaatst (komt van vr 9:55u) en de docent is vervangen (was srs).",
			[]string{"teacher", "time"}, "nl",
			map[string]string{"teacher": "srs", "time": "vr 9:55"}, "",
		},
		// A clause Melon does not know stays, the known one still moves to its chip.
		{
			"Het lokaal is gewijzigd (was 108) en de toets is ingekort.",
			[]string{"location"}, "nl",
			map[string]string{"location": "108"}, "De toets is ingekort.",
		},
		// Nothing recognised: the note is kept exactly.
		{"Les gaat online door.", []string{"location"}, "nl", map[string]string{}, "Les gaat online door."},
		// A clause about a change Zermelo did not flag is not trusted.
		{"Het lokaal is gewijzigd (was 127).", []string{"teacher"}, "nl", map[string]string{}, "Het lokaal is gewijzigd (was 127)."},
		{"", []string{"location"}, "nl", map[string]string{}, ""},
	} {
		prev, rest := parseChangeNote(tc.note, tc.changes, tc.lang)
		if !maps.Equal(prev, tc.prev) || rest != tc.rest {
			t.Errorf("%q: got %v %q, want %v %q", tc.note, prev, rest, tc.prev, tc.rest)
		}
	}
}
