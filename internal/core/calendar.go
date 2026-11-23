package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// CalendarToken returns the account's calendar feed secret, or "" when the
// feed is off. Only browser sessions may read it.
func (s *Service) CalendarToken(ctx context.Context, p Principal, accountID string) (string, error) {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil || a.CalEnc == nil {
		return "", err
	}
	return s.Box.Open(a.CalEnc, "cal:"+a.ID)
}

// EnableCalendar creates a fresh feed secret; any previous URL stops working.
func (s *Service) EnableCalendar(ctx context.Context, p Principal, accountID string) (string, error) {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return "", err
	}
	token := secret.Token("")
	hash := secret.Hash(token)
	return token, s.Store.SetCalendar(ctx, p.UserID, a.ID, &hash, s.Box.Seal(token, "cal:"+a.ID))
}

func (s *Service) DisableCalendar(ctx context.Context, p Principal, accountID string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	return s.Store.SetCalendar(ctx, p.UserID, a.ID, nil, nil)
}

// CalendarFeed renders the iCalendar feed behind a feed secret: last week
// through the next five. Calendar apps poll rarely, so it may be cached longer.
// It is built anew on every request; only the Zermelo data under it is cached.
// base is Melon's own origin, for the link that opens each lesson in the app.
func (s *Service) CalendarFeed(ctx context.Context, token, lang, base string) (string, error) {
	a, err := s.Store.AccountByCalendar(ctx, secret.Hash(token))
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	p := Principal{UserID: a.UserID, Kind: "calendar", AccountIDs: []string{a.ID}}
	from := mondayOf(s.Today()).AddDate(0, 0, -7)
	sched, err := s.Schedule(ctx, p, a.ID, ScheduleQuery{From: from, To: from.AddDate(0, 0, MaxRangeDays-1), Lang: lang, MaxAge: 10 * time.Minute})
	if err != nil {
		return "", err
	}

	nl := lang != "en"
	word := func(dutch, english string) string {
		if nl {
			return dutch
		}
		return english
	}
	var b strings.Builder
	line := func(format string, args ...any) { foldLine(&b, fmt.Sprintf(format, args...)) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Melon//Schedule//%s", strings.ToUpper(word("nl", "en")))
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:%s", icsText(word("Rooster ", "Schedule ")+a.DisplayName))
	line("X-WR-TIMEZONE:%s", s.Loc.String())
	line("REFRESH-INTERVAL;VALUE=DURATION:PT30M")
	line("X-PUBLISHED-TTL:PT30M")
	const stamp = "20060102T150405Z"
	for _, l := range sched.Lessons {
		// Dropped-out (cancelled) lessons are left out of the feed entirely.
		// The feed is rebuilt from the schedule on every fetch and covers a
		// fixed window, so subscribers lose them on refresh, past ones too.
		if l.Status == "cancelled" {
			continue
		}
		summary := l.Title
		if len(l.Locations) > 0 {
			summary += " · " + strings.Join(l.Locations, ", ")
		}
		var desc []string
		switch l.Status {
		case "changed":
			summary = word("Gewijzigd: ", "Changed: ") + summary
		}
		if l.ChangeNote != "" {
			desc = append(desc, l.ChangeNote)
		}
		if l.Note != "" {
			desc = append(desc, l.Note)
		}
		if len(l.Teachers) > 0 {
			var who []string
			for _, t := range l.Teachers {
				who = append(who, t.Name)
			}
			desc = append(desc, word("Docent: ", "Teacher: ")+strings.Join(who, ", "))
		}
		if len(l.Groups) > 0 {
			desc = append(desc, word("Groep: ", "Group: ")+strings.Join(l.Groups, ", "))
		}
		if l.Period != nil {
			desc = append(desc, fmt.Sprintf(word("%de uur", "Period %d"), l.Period.From))
		}
		if l.WeekPlan != "" {
			desc = append(desc, word("Weekplanning: ", "Week plan: ")+l.WeekPlan)
		}
		for _, h := range l.Homework {
			head := word("Huiswerk:", "Homework:")
			if h.Done {
				head = word("✓ Huiswerk (af):", "✓ Homework (done):")
			}
			desc = append(desc, "", head, plainText(h.Text))
		}
		// A link straight to this lesson in Melon. Calendar apps make bare
		// URLs in the description clickable; URL is there for the ones that
		// show it as a field (Apple Calendar).
		open := lessonLink(base, a.ID, l)
		desc = append(desc, "", word("Openen in Melon: ", "Open in Melon: ")+open)

		// Homework and week plans change without Zermelo knowing, so the
		// newest of all three is what tells calendar apps to update the event.
		modified := sched.FetchedAt
		if l.UpdatedAt != nil {
			modified = *l.UpdatedAt
		}
		if l.weekPlanUpdated != nil && l.weekPlanUpdated.After(modified) {
			modified = *l.weekPlanUpdated
		}
		for _, h := range l.Homework {
			if h.UpdatedAt.After(modified) {
				modified = h.UpdatedAt
			}
		}
		line("BEGIN:VEVENT")
		line("UID:%s-%s@melon", a.ID, l.ID)
		line("DTSTAMP:%s", modified.UTC().Format(stamp))
		line("LAST-MODIFIED:%s", modified.UTC().Format(stamp))
		line("DTSTART:%s", l.Start.UTC().Format(stamp))
		line("DTEND:%s", l.End.UTC().Format(stamp))
		line("SUMMARY:%s", icsText(summary))
		line("URL:%s", open)
		if len(l.Locations) > 0 {
			line("LOCATION:%s", icsText(strings.Join(l.Locations, ", ")))
		}
		if len(desc) > 0 {
			line("DESCRIPTION:%s", icsText(strings.Join(desc, "\n")))
		}
		line("STATUS:CONFIRMED")
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String(), nil
}

// lessonLink opens a lesson in the web app: the account, its day, and the
// lesson by instance, so the link still works after Zermelo moves it.
func lessonLink(base, accountID string, l Lesson) string {
	q := url.Values{"account": {accountID}, "day": {l.Date}, "lesson": {l.Instance}}
	return strings.TrimSuffix(base, "/") + "/?" + q.Encode()
}

var (
	mdTask   = regexp.MustCompile(`^(\s*)(?:[-*]|\d+\.)\s+\[([ xX])\]\s+`)
	mdBullet = regexp.MustCompile(`^(\s*)[-*]\s+`)
	mdLink   = regexp.MustCompile(`\[([^\]]+)\]\((\S+?)\)`)
	mdMarks  = strings.NewReplacer("**", "", "~~", "", "`", "")
	mdItalic = regexp.MustCompile(`\*([^*\s][^*]*?)\*`)
)

// plainText turns homework Markdown into text a calendar app shows well:
// checklists as ☐ and ☑, bullets as •, links as "text (url)" so the URL
// stays clickable, and no ** or # left over. Calendar descriptions are plain
// text (RFC 5545); Google Calendar ignores the HTML alternative, X-ALT-DESC.
func plainText(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	for i, l := range lines {
		if m := mdTask.FindStringSubmatch(l); m != nil {
			box := "☐ "
			if m[2] != " " {
				box = "☑ "
			}
			l = m[1] + box + l[len(m[0]):]
		} else if m := mdBullet.FindStringSubmatch(l); m != nil {
			l = m[1] + "• " + l[len(m[0]):]
		}
		l = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(l, "### "), "## "), "# "), "> ")
		l = mdLink.ReplaceAllStringFunc(l, func(s string) string {
			m := mdLink.FindStringSubmatch(s)
			if m[1] == m[2] {
				return m[2]
			}
			return m[1] + " (" + m[2] + ")"
		})
		l = mdItalic.ReplaceAllString(mdMarks.Replace(l), "$1")
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

func icsText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "").Replace(s)
}

// foldLine writes one content line, folded at 75 octets as RFC 5545 requires,
// without splitting a UTF-8 sequence.
func foldLine(b *strings.Builder, s string) {
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && s[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(s[:cut])
		b.WriteString("\r\n ")
		s = s[cut:]
		limit = 74
	}
	b.WriteString(s)
	b.WriteString("\r\n")
}
