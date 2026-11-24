package subjects

import "testing"

func TestTestSubjects(t *testing.T) {
	global := map[string]Name{"xyz": {NL: "Xylofoon", EN: ""}}
	for code, want := range map[string][2]string{
		"ttsmu":  {"Toets muziek", "Music test"},
		"TTSMU2": {"Toets muziek", "Music test"},
		"ttsen":  {"Toets Engels", "English test"},
		"ttswa":  {"Toets wiskunde A", "Mathematics A test"},
		"ttsxyz": {"Toets xylofoon", "Toets xylofoon"},
		"ttsqq":  {"ttsqq", "ttsqq"},
		"tts":    {"tts", "tts"},
	} {
		nl, _ := Resolve(code, "nl", nil, global)
		en, _ := Resolve(code, "en", nil, global)
		if nl != want[0] || en != want[1] {
			t.Errorf("%s: got %q / %q, want %q / %q", code, nl, en, want[0], want[1])
		}
	}
	// A name set for the exact code wins.
	if n, _ := Resolve("ttsmu", "nl", map[string]string{"ttsmu": "Muziektoets"}, nil); n != "Muziektoets" {
		t.Errorf("override: %q", n)
	}
}
