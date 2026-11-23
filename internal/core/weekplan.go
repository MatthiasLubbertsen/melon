package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var weekPattern = regexp.MustCompile(`^\d{4}-W(0[1-9]|[1-4]\d|5[0-3])$`)

func isoWeekOf(l Lesson) string {
	y, w := l.Start.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// attachWeekPlans puts the week plan link saved for a lesson's subject and
// week on it.
func (s *Service) attachWeekPlans(ctx context.Context, accountID string, out *Schedule) error {
	seen := map[string]bool{}
	var weeks []string
	for _, l := range out.Lessons {
		if w := isoWeekOf(l); !seen[w] {
			seen[w] = true
			weeks = append(weeks, w)
		}
	}
	plans, err := s.Store.WeekPlans(ctx, accountID, weeks)
	if err != nil || len(plans) == 0 {
		return err
	}
	for i := range out.Lessons {
		l := &out.Lessons[i]
		for _, code := range subjectCodes(*l) {
			if u := plans[code+"|"+isoWeekOf(*l)]; u != "" {
				l.WeekPlan = u
				break
			}
		}
	}
	return nil
}

func cleanPlanURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(raw) > 1000 {
		return "", Errf(http.StatusBadRequest, "invalid_url", "The week plan must be a web address starting with https://.")
	}
	return raw, nil
}

// SetWeekPlan saves a link for one subject in one ISO week. An empty url
// removes it.
func (s *Service) SetWeekPlan(ctx context.Context, p Principal, accountID, subject, week, link string) (string, error) {
	if _, _, err := s.account(ctx, p, accountID); err != nil {
		return "", err
	}
	subject = strings.ToLower(strings.TrimSpace(subject))
	week = strings.ToUpper(strings.TrimSpace(week))
	if subject == "" || len(subject) > 32 {
		return "", Errf(http.StatusBadRequest, "invalid_request", "That is not a subject code.")
	}
	if !weekPattern.MatchString(week) {
		return "", Errf(http.StatusBadRequest, "invalid_request", "'week' must look like 2026-W48.")
	}
	if strings.TrimSpace(link) == "" {
		return "", s.Store.DeleteWeekPlan(ctx, accountID, subject, week)
	}
	link, err := cleanPlanURL(link)
	if err != nil {
		return "", err
	}
	return link, s.Store.SetWeekPlan(ctx, accountID, subject, week, link)
}
