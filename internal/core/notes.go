package core

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// Limits on notes and board cards, in characters.
const (
	MaxNoteLength      = 20000
	MaxNoteTitleLength = 200
	MaxCardTitleLength = 200
	MaxCardTextLength  = 5000
)

// Note is a Markdown note kept in Melon. Subject is a subject code
// (lower case) for a note about one subject, or empty for a general note.
type Note struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Boards and their lanes, left to right. The Hackban is a board in the
// spirit of Hack Club: from an idea, through hacking on it, to shipping it.
var Boards = map[string][]string{
	"kanban":  {"todo", "doing", "done"},
	"hackban": {"idea", "hacking", "shipped"},
}

// Card is one card on a board. Text is Markdown; Subject optionally ties it
// to a subject code.
type Card struct {
	ID        string    `json:"id"`
	Board     string    `json:"board"`
	Lane      string    `json:"lane"`
	Position  int       `json:"position"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Service) toNote(n store.Note) Note {
	return Note{ID: n.ID, Subject: n.Subject, Title: n.Title, Text: n.Text, Pinned: n.Pinned, CreatedAt: time.Unix(n.CreatedAt, 0).In(s.Loc), UpdatedAt: time.Unix(n.UpdatedAt, 0).In(s.Loc)}
}

func (s *Service) toCard(c store.Card) Card {
	return Card{ID: c.ID, Board: c.Board, Lane: c.Lane, Position: c.Position, Title: c.Title, Text: c.Text, Subject: c.Subject, CreatedAt: time.Unix(c.CreatedAt, 0).In(s.Loc), UpdatedAt: time.Unix(c.UpdatedAt, 0).In(s.Loc)}
}

func (s *Service) notesAccount(ctx context.Context, p Principal, accountID string) error {
	_, _, err := s.account(ctx, p, accountID)
	return err
}

func cleanText(field, text string, max int, multiline bool) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if !multiline {
		text = strings.Join(strings.Fields(text), " ")
	}
	if utf8.RuneCountInString(text) > max {
		return "", Errf(http.StatusBadRequest, "invalid_request", "'%s' can be at most %d characters.", field, max)
	}
	return text, nil
}

func cleanSubject(code string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) > 32 || strings.ContainsAny(code, " \t\n/") {
		return "", Errf(http.StatusBadRequest, "invalid_request", "'subject' must be a subject code, like 'du', or empty.")
	}
	return code, nil
}

// NoteInput is a new note, or a partial update of one (nil stays the same).
type NoteInput struct {
	Subject *string
	Title   *string
	Text    *string
	Pinned  *bool
}

func applyNote(n *store.Note, in NoteInput) (err error) {
	if in.Subject != nil {
		if n.Subject, err = cleanSubject(*in.Subject); err != nil {
			return err
		}
	}
	if in.Title != nil {
		if n.Title, err = cleanText("title", *in.Title, MaxNoteTitleLength, false); err != nil {
			return err
		}
	}
	if in.Text != nil {
		if n.Text, err = cleanText("text", *in.Text, MaxNoteLength, true); err != nil {
			return err
		}
	}
	if in.Pinned != nil {
		n.Pinned = *in.Pinned
	}
	if n.Title == "" && n.Text == "" {
		return Errf(http.StatusBadRequest, "invalid_request", "A note needs a title or some text.")
	}
	return nil
}

// Notes lists an account's notes. subject nil lists every note, "" only the
// general ones, a code only those about that subject.
func (s *Service) Notes(ctx context.Context, p Principal, accountID string, subject *string) ([]Note, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return nil, err
	}
	if subject != nil {
		code, err := cleanSubject(*subject)
		if err != nil {
			return nil, err
		}
		subject = &code
	}
	list, err := s.Store.Notes(ctx, accountID, subject)
	if err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(list))
	for _, n := range list {
		out = append(out, s.toNote(n))
	}
	return out, nil
}

func (s *Service) AddNote(ctx context.Context, p Principal, accountID string, in NoteInput) (Note, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return Note{}, err
	}
	n := store.Note{ID: secret.ID("note"), AccountID: accountID}
	if err := applyNote(&n, in); err != nil {
		return Note{}, err
	}
	n, err := s.Store.CreateNote(ctx, n)
	return s.toNote(n), err
}

func (s *Service) UpdateNote(ctx context.Context, p Principal, accountID, id string, in NoteInput) (Note, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return Note{}, err
	}
	n, err := s.Store.Note(ctx, accountID, id)
	if errors.Is(err, store.ErrNotFound) {
		return Note{}, ErrNotFound
	} else if err != nil {
		return Note{}, err
	}
	if err := applyNote(&n, in); err != nil {
		return Note{}, err
	}
	n, err = s.Store.UpdateNote(ctx, n)
	return s.toNote(n), err
}

func (s *Service) DeleteNote(ctx context.Context, p Principal, accountID, id string) error {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return err
	}
	if err := s.Store.DeleteNote(ctx, accountID, id); errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	} else {
		return err
	}
}

func checkBoard(board string) error {
	if _, ok := Boards[board]; !ok {
		return Errf(http.StatusBadRequest, "invalid_request", "'board' must be 'kanban' or 'hackban'.")
	}
	return nil
}

// Cards is every card on one of the account's boards, by lane and position.
func (s *Service) Cards(ctx context.Context, p Principal, accountID, board string) ([]Card, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return nil, err
	}
	if err := checkBoard(board); err != nil {
		return nil, err
	}
	list, err := s.Store.Cards(ctx, accountID, board)
	if err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(list))
	for _, c := range list {
		out = append(out, s.toCard(c))
	}
	// Lanes left to right, as the board shows them.
	lanes := Boards[board]
	slices.SortStableFunc(out, func(a, b Card) int { return slices.Index(lanes, a.Lane) - slices.Index(lanes, b.Lane) })
	return out, nil
}

// CardInput is a new card, or a partial update of one (nil stays the same).
// Index is where in its lane the card goes; nil keeps it where it is, or
// puts a new card last.
type CardInput struct {
	Board   string
	Lane    *string
	Index   *int
	Title   *string
	Text    *string
	Subject *string
}

func applyCard(c *store.Card, in CardInput) (err error) {
	if in.Lane != nil {
		if !slices.Contains(Boards[c.Board], *in.Lane) {
			return Errf(http.StatusBadRequest, "invalid_request", "'lane' must be one of %s on the %s board.", strings.Join(Boards[c.Board], ", "), c.Board)
		}
		c.Lane = *in.Lane
	}
	if in.Title != nil {
		if c.Title, err = cleanText("title", *in.Title, MaxCardTitleLength, false); err != nil {
			return err
		}
	}
	if in.Text != nil {
		if c.Text, err = cleanText("text", *in.Text, MaxCardTextLength, true); err != nil {
			return err
		}
	}
	if in.Subject != nil {
		if c.Subject, err = cleanSubject(*in.Subject); err != nil {
			return err
		}
	}
	if c.Title == "" {
		return Errf(http.StatusBadRequest, "invalid_request", "A card needs a title.")
	}
	return nil
}

func (s *Service) AddCard(ctx context.Context, p Principal, accountID string, in CardInput) (Card, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return Card{}, err
	}
	if err := checkBoard(in.Board); err != nil {
		return Card{}, err
	}
	c := store.Card{ID: secret.ID("card"), AccountID: accountID, Board: in.Board, Lane: Boards[in.Board][0]}
	if err := applyCard(&c, in); err != nil {
		return Card{}, err
	}
	index := -1
	if in.Index != nil {
		index = *in.Index
	}
	c, err := s.Store.SaveCard(ctx, c, index)
	return s.toCard(c), err
}

func (s *Service) UpdateCard(ctx context.Context, p Principal, accountID, id string, in CardInput) (Card, error) {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return Card{}, err
	}
	c, err := s.Store.Card(ctx, accountID, id)
	if errors.Is(err, store.ErrNotFound) {
		return Card{}, ErrNotFound
	} else if err != nil {
		return Card{}, err
	}
	lane := c.Lane
	if err := applyCard(&c, in); err != nil {
		return Card{}, err
	}
	index := c.Position
	switch {
	case in.Index != nil:
		index = *in.Index
	case c.Lane != lane:
		index = -1 // moved to another lane without a place: last
	}
	c, err = s.Store.SaveCard(ctx, c, index)
	return s.toCard(c), err
}

func (s *Service) DeleteCard(ctx context.Context, p Principal, accountID, id string) error {
	if err := s.notesAccount(ctx, p, accountID); err != nil {
		return err
	}
	if err := s.Store.DeleteCard(ctx, accountID, id); errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	} else {
		return err
	}
}
