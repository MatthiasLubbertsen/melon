package store

import (
	"context"
	"strings"
)

// Week plans are links (say, an itslearning plan) someone saved for one
// subject in one ISO week ("2026-W48"), shown on every lesson of it.
const weekPlanSchema = `
CREATE TABLE IF NOT EXISTS week_plans (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  subject TEXT NOT NULL, week TEXT NOT NULL, url TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, subject, week)
);
`

// WeekPlans returns the links saved for the given weeks, keyed "subject|week".
func (s *Store) WeekPlans(ctx context.Context, accountID string, weeks []string) (map[string]string, error) {
	out := map[string]string{}
	if len(weeks) == 0 {
		return out, nil
	}
	args := []any{accountID}
	for _, w := range weeks {
		args = append(args, w)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT subject, week, url FROM week_plans WHERE account_id = ? AND week IN (?`+strings.Repeat(",?", len(weeks)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var subject, week, url string
		if err := rows.Scan(&subject, &week, &url); err != nil {
			return nil, err
		}
		out[subject+"|"+week] = url
	}
	return out, rows.Err()
}

func (s *Store) SetWeekPlan(ctx context.Context, accountID, subject, week, url string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO week_plans (account_id, subject, week, url, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT (account_id, subject, week) DO UPDATE SET url = excluded.url, updated_at = excluded.updated_at`, accountID, subject, week, url, now())
	return err
}

func (s *Store) DeleteWeekPlan(ctx context.Context, accountID, subject, week string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM week_plans WHERE account_id = ? AND subject = ? AND week = ?`, accountID, subject, week)
	return err
}
