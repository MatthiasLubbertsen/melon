package store

import "context"

const detentionSchema = `
CREATE TABLE IF NOT EXISTS detentions (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  date TEXT NOT NULL, start TEXT NOT NULL, end TEXT NOT NULL,
  period_from INTEGER NOT NULL DEFAULT 0, period_to INTEGER NOT NULL DEFAULT 0,
  subject TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', location TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS detentions_day ON detentions(account_id, date);
`

// Detention is an hour someone has to stay at school that Zermelo does not
// know about, added by the account itself. Times are school-local "15:30".
type Detention struct {
	ID        string
	AccountID string
	Date      string
	Start     string
	End       string
	// PeriodFrom and PeriodTo are 0 when it was entered by time only.
	PeriodFrom int
	PeriodTo   int
	// Subject is a Zermelo subject code it belongs to, or empty.
	Subject   string
	Title     string
	Location  string
	CreatedAt int64
}

// Detentions lists an account's detentions from one date through another, inclusive.
func (s *Store) Detentions(ctx context.Context, accountID, from, to string) ([]Detention, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, date, start, end, period_from, period_to, subject, title, location, created_at
		FROM detentions WHERE account_id = ? AND date >= ? AND date <= ? ORDER BY date, start`, accountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Detention{}
	for rows.Next() {
		var d Detention
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Date, &d.Start, &d.End, &d.PeriodFrom, &d.PeriodTo, &d.Subject, &d.Title, &d.Location, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AddDetention(ctx context.Context, d Detention) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO detentions (id, account_id, date, start, end, period_from, period_to, subject, title, location, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.ID, d.AccountID, d.Date, d.Start, d.End, d.PeriodFrom, d.PeriodTo, d.Subject, d.Title, d.Location, now())
	return err
}

func (s *Store) DeleteDetention(ctx context.Context, accountID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM detentions WHERE account_id = ? AND id = ?`, accountID, id))
}
