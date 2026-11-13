package core

import (
	"context"
	"errors"
	"fmt"
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
func (s *Service) CalendarFeed(ctx context.Context, token, lang string) (string, error) {
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
		summary := l.Title
		if len(l.Locations) > 0 {
			summary += " · " + strings.Join(l.Locations, ", ")
		}
		var desc []string
		switch l.Status {
		case "cancelled":
			summary = word("Vervallen: ", "Cancelled: ") + summary
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
			desc = append(desc, word("Docent: ", "Teacher: ")+strings.Join(l.Teachers, ", "))
		}
		if len(l.Groups) > 0 {
			desc = append(desc, word("Groep: ", "Group: ")+strings.Join(l.Groups, ", "))
		}
		if l.Period != nil {
			desc = append(desc, fmt.Sprintf(word("%de uur", "Period %d"), l.Period.From))
		}
		modified := sched.FetchedAt
		if l.UpdatedAt != nil {
			modified = *l.UpdatedAt
		}
		line("BEGIN:VEVENT")
		line("UID:%s-%s@melon", a.ID, l.ID)
		line("DTSTAMP:%s", modified.UTC().Format(stamp))
		line("LAST-MODIFIED:%s", modified.UTC().Format(stamp))
		line("DTSTART:%s", l.Start.UTC().Format(stamp))
		line("DTEND:%s", l.End.UTC().Format(stamp))
		line("SUMMARY:%s", icsText(summary))
		if len(l.Locations) > 0 {
			line("LOCATION:%s", icsText(strings.Join(l.Locations, ", ")))
		}
		if len(desc) > 0 {
			line("DESCRIPTION:%s", icsText(strings.Join(desc, "\n")))
		}
		if l.Status == "cancelled" {
			line("STATUS:CANCELLED")
			line("TRANSP:TRANSPARENT")
		} else {
			line("STATUS:CONFIRMED")
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String(), nil
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
