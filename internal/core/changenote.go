package core

import (
	"regexp"
	"slices"
	"strings"
)

// Zermelo describes a change in a generated Dutch sentence, one clause per
// change joined with "en": "Het lokaal is gewijzigd (was 108) en de les is
// verplaatst (komt van vr 11:55u)." The clauses below are the ones seen in the
// wild; each yields the old value of one change.
var changeClauses = []struct {
	change string
	re     *regexp.Regexp
}{
	{"location", regexp.MustCompile(`(?i)^het lokaal is (?:gewijzigd|veranderd) \(was (.+)\)$`)},
	{"teacher", regexp.MustCompile(`(?i)^de docent is (?:gewijzigd|vervangen|veranderd) \(was (.+)\)$`)},
	{"teacher", regexp.MustCompile(`(?i)^er is een wijziging bij docenten: (.+) wordt .+$`)},
	{"group", regexp.MustCompile(`(?i)^de groep is (?:gewijzigd|veranderd) \(was (.+)\)$`)},
	{"time", regexp.MustCompile(`(?i)^de (?:les|activiteit|afspraak|toets) is verplaatst \((?:komt van|was) (.+)\)$`)},
}

// A new clause starts at "en de", "en het" or "en er".
var clauseJoin = regexp.MustCompile(`(?i)\s+en\s+((?:de|het|er)\s)`)

var (
	hourSuffix = regexp.MustCompile(`(\d{1,2}:\d{2})u\b`)
	listComma  = regexp.MustCompile(`\s*,\s*`)
)

var englishDays = map[string]string{"ma": "Mon", "di": "Tue", "wo": "Wed", "do": "Thu", "vr": "Fri", "za": "Sat", "zo": "Sun"}

// parseChangeNote reads the old value of each change out of Zermelo's note.
// It returns those values and the clauses it could not place, so nothing the
// note says gets lost. Only changes listed in changes are taken from the note.
func parseChangeNote(note string, changes []string, lang string) (map[string]string, string) {
	prev := map[string]string{}
	note = strings.TrimSpace(note)
	if note == "" {
		return prev, ""
	}
	var clauses []string
	at := 0
	for _, m := range clauseJoin.FindAllStringSubmatchIndex(note, -1) {
		clauses = append(clauses, note[at:m[0]])
		at = m[2] // the next clause keeps its "de", "het" or "er"
	}
	clauses = append(clauses, note[at:])

	var rest []string
	for _, c := range clauses {
		c = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(c), "."))
		placed := false
		for _, cl := range changeClauses {
			m := cl.re.FindStringSubmatch(c)
			if m == nil || !slices.Contains(changes, cl.change) || prev[cl.change] != "" {
				continue
			}
			prev[cl.change] = tidyOld(cl.change, m[1], lang)
			placed = true
			break
		}
		if !placed && c != "" {
			rest = append(rest, c)
		}
	}
	if len(rest) == 0 {
		return prev, ""
	}
	if len(rest) == len(clauses) {
		return prev, note
	}
	out := strings.Join(rest, " en ")
	return prev, strings.ToUpper(out[:1]) + out[1:] + "."
}

func tidyOld(change, v, lang string) string {
	v = strings.TrimSpace(v)
	switch change {
	case "teacher":
		v = listComma.ReplaceAllString(v, ", ")
	case "time":
		v = hourSuffix.ReplaceAllString(v, "$1")
		if lang == "en" {
			if day, t, ok := strings.Cut(v, " "); ok && englishDays[strings.ToLower(day)] != "" {
				v = englishDays[strings.ToLower(day)] + " " + t
			}
		}
	}
	return v
}
