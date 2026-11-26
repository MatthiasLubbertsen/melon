package store

import (
	"context"
	"database/sql"
)

// Notes are Markdown notes someone keeps in Melon: general ones (Subject is
// empty) or ones about one subject (a Zermelo subject code, lower case).
//
// Cards are the cards on an account's boards: "kanban" (todo, doing, done)
// and "hackban" (idea, hacking, shipped). Position orders the cards within
// one lane.
const notesSchema = `
CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  subject TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  pinned INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS notes_account ON notes(account_id, subject);
CREATE TABLE IF NOT EXISTS board_cards (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  board TEXT NOT NULL, lane TEXT NOT NULL, position INTEGER NOT NULL,
  title TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', subject TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS board_cards_account ON board_cards(account_id, board, lane, position);
`

type Note struct {
	ID        string
	AccountID string
	Subject   string
	Title     string
	Text      string
	Pinned    bool
	CreatedAt int64
	UpdatedAt int64
}

const noteColumns = `id, account_id, subject, title, text, pinned, created_at, updated_at`

func scanNote(row interface{ Scan(...any) error }) (Note, error) {
	var n Note
	err := row.Scan(&n.ID, &n.AccountID, &n.Subject, &n.Title, &n.Text, &n.Pinned, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

// Notes lists the account's notes, pinned first, then the latest changed.
// subject nil means every note; "" only the general ones.
func (s *Store) Notes(ctx context.Context, accountID string, subject *string) ([]Note, error) {
	q := `SELECT ` + noteColumns + ` FROM notes WHERE account_id = ?`
	args := []any{accountID}
	if subject != nil {
		q += ` AND subject = ?`
		args = append(args, *subject)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY pinned DESC, updated_at DESC, created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Note(ctx context.Context, accountID, id string) (Note, error) {
	n, err := scanNote(s.db.QueryRowContext(ctx, `SELECT `+noteColumns+` FROM notes WHERE account_id = ? AND id = ?`, accountID, id))
	return n, one(err)
}

func (s *Store) CreateNote(ctx context.Context, n Note) (Note, error) {
	n.CreatedAt, n.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO notes (`+noteColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		n.ID, n.AccountID, n.Subject, n.Title, n.Text, n.Pinned, n.CreatedAt, n.UpdatedAt)
	return n, err
}

func (s *Store) UpdateNote(ctx context.Context, n Note) (Note, error) {
	n.UpdatedAt = now()
	return n, affected(s.db.ExecContext(ctx, `UPDATE notes SET subject = ?, title = ?, text = ?, pinned = ?, updated_at = ? WHERE account_id = ? AND id = ?`,
		n.Subject, n.Title, n.Text, n.Pinned, n.UpdatedAt, n.AccountID, n.ID))
}

func (s *Store) DeleteNote(ctx context.Context, accountID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM notes WHERE account_id = ? AND id = ?`, accountID, id))
}

type Card struct {
	ID        string
	AccountID string
	Board     string
	Lane      string
	Position  int
	Title     string
	Text      string
	Subject   string
	CreatedAt int64
	UpdatedAt int64
}

const cardColumns = `id, account_id, board, lane, position, title, text, subject, created_at, updated_at`

func scanCard(row interface{ Scan(...any) error }) (Card, error) {
	var c Card
	err := row.Scan(&c.ID, &c.AccountID, &c.Board, &c.Lane, &c.Position, &c.Title, &c.Text, &c.Subject, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func cardRows(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, where string, args ...any) ([]Card, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+cardColumns+` FROM board_cards `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Cards is every card on one board, by lane and position.
func (s *Store) Cards(ctx context.Context, accountID, board string) ([]Card, error) {
	return cardRows(ctx, s.db, `WHERE account_id = ? AND board = ? ORDER BY lane, position`, accountID, board)
}

func (s *Store) Card(ctx context.Context, accountID, id string) (Card, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx, `SELECT `+cardColumns+` FROM board_cards WHERE account_id = ? AND id = ?`, accountID, id))
	return c, one(err)
}

// SaveCard writes a card (new or changed) and puts it at index in its lane,
// renumbering the lane. An index past the end, or below zero, means last.
func (s *Store) SaveCard(ctx context.Context, c Card, index int) (Card, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	lane, err := cardRows(ctx, tx, `WHERE account_id = ? AND board = ? AND lane = ? AND id != ? ORDER BY position`, c.AccountID, c.Board, c.Lane, c.ID)
	if err != nil {
		return c, err
	}
	if index < 0 || index > len(lane) {
		index = len(lane)
	}
	t := now()
	if c.CreatedAt == 0 {
		c.CreatedAt = t
	}
	c.UpdatedAt = t
	c.Position = index
	if _, err := tx.ExecContext(ctx, `INSERT INTO board_cards (`+cardColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET lane = excluded.lane, position = excluded.position, title = excluded.title, text = excluded.text, subject = excluded.subject, updated_at = excluded.updated_at`,
		c.ID, c.AccountID, c.Board, c.Lane, c.Position, c.Title, c.Text, c.Subject, c.CreatedAt, c.UpdatedAt); err != nil {
		return c, err
	}
	for i, other := range lane {
		pos := i
		if i >= index {
			pos = i + 1
		}
		if other.Position != pos {
			if _, err := tx.ExecContext(ctx, `UPDATE board_cards SET position = ? WHERE id = ?`, pos, other.ID); err != nil {
				return c, err
			}
		}
	}
	return c, tx.Commit()
}

func (s *Store) DeleteCard(ctx context.Context, accountID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM board_cards WHERE account_id = ? AND id = ?`, accountID, id))
}
