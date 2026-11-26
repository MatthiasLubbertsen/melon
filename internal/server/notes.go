package server

import (
	"context"
	"net/http"

	"github.com/matthiaslubbertsen/melon/internal/core"
)

type noteBody struct {
	Subject *string `json:"subject"`
	Title   *string `json:"title"`
	Text    *string `json:"text"`
	Pinned  *bool   `json:"pinned"`
}

func (b noteBody) input() core.NoteInput {
	return core.NoteInput{Subject: b.Subject, Title: b.Title, Text: b.Text, Pinned: b.Pinned}
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var subject *string
	if r.URL.Query().Has("subject") {
		v := r.URL.Query().Get("subject")
		subject = &v
	}
	list, err := s.Core.Notes(r.Context(), p, r.PathValue("account"), subject)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, list)
}

func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body noteBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	n, err := s.Core.AddNote(r.Context(), p, r.PathValue("account"), body.input())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": n})
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body noteBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	n, err := s.Core.UpdateNote(r.Context(), p, r.PathValue("account"), r.PathValue("id"), body.input())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, n)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.DeleteNote(r.Context(), p, r.PathValue("account"), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type cardBody struct {
	Lane    *string `json:"lane"`
	Index   *int    `json:"index"`
	Title   *string `json:"title"`
	Text    *string `json:"text"`
	Subject *string `json:"subject"`
}

func (b cardBody) input(board string) core.CardInput {
	return core.CardInput{Board: board, Lane: b.Lane, Index: b.Index, Title: b.Title, Text: b.Text, Subject: b.Subject}
}

func (s *Server) handleCards(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, err := s.Core.Cards(r.Context(), p, r.PathValue("account"), r.PathValue("board"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, list)
}

func (s *Server) handleAddCard(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body cardBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.Core.AddCard(r.Context(), p, r.PathValue("account"), body.input(r.PathValue("board")))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": c})
}

func (s *Server) handleUpdateCard(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body cardBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.Core.UpdateCard(r.Context(), p, r.PathValue("account"), r.PathValue("id"), body.input(""))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, c)
}

func (s *Server) handleDeleteCard(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.DeleteCard(r.Context(), p, r.PathValue("account"), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var (
	writes      = map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false}
	deletes     = map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": false}
	boardProp   = map[string]any{"type": "string", "enum": []string{"kanban", "hackban"}, "description": "'kanban' (lanes todo, doing, done) or 'hackban' (lanes idea, hacking, shipped)."}
	subjectProp = str("Subject code, e.g. 'du', or empty for none.")
)

func init() {
	mcpTools = append(mcpTools,
		mcpTool{
			Name: "list_notes", Title: "List notes",
			Description: "List the user's notes in Melon (Markdown): general notes (subject '') and notes about one subject (a subject code). Pinned first, then the latest changed. Leave out subject for all notes.",
			InputSchema: schema(nil, map[string]any{"account": accountProp, "subject": str("Only notes about this subject code; '' for only general notes.")}), Annotations: readOnly,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				list, err := s.Core.Notes(ctx, p, id, a.Subject)
				return map[string]any{"notes": list}, err
			},
		},
		mcpTool{
			Name: "add_note", Title: "Add a note",
			Description: "Write a note in Melon. Text is Markdown. Give a subject code for a note about one subject, or leave it out for a general note.",
			InputSchema: schema(nil, map[string]any{"account": accountProp, "subject": subjectProp, "title": str("Short title."), "text": str("The note, in Markdown."), "pinned": map[string]any{"type": "boolean"}}),
			Annotations: writes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.AddNote(ctx, p, id, core.NoteInput{Subject: a.Subject, Title: a.Title, Text: a.Text, Pinned: a.Pinned})
			},
		},
		mcpTool{
			Name: "update_note", Title: "Update a note",
			Description: "Change a note's title, text (Markdown), subject or pin. Leave out what stays the same.",
			InputSchema: schema([]string{"id"}, map[string]any{"account": accountProp, "id": str("Note id from list_notes."), "subject": subjectProp, "title": str("Short title."), "text": str("The note, in Markdown."), "pinned": map[string]any{"type": "boolean"}}),
			Annotations: writes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.UpdateNote(ctx, p, id, a.ID, core.NoteInput{Subject: a.Subject, Title: a.Title, Text: a.Text, Pinned: a.Pinned})
			},
		},
		mcpTool{
			Name: "delete_note", Title: "Delete a note",
			Description: "Delete a note. This cannot be undone.",
			InputSchema: schema([]string{"id"}, map[string]any{"account": accountProp, "id": str("Note id from list_notes.")}),
			Annotations: deletes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": a.ID, "deleted": true}, s.Core.DeleteNote(ctx, p, id, a.ID)
			},
		},
		mcpTool{
			Name: "list_cards", Title: "List board cards",
			Description: "List the cards on one of the user's boards in Melon, by lane (left to right) and position. The kanban board has lanes todo, doing, done; the hackban board (for side projects, Hack Club style) has idea, hacking, shipped.",
			InputSchema: schema([]string{"board"}, map[string]any{"account": accountProp, "board": boardProp}), Annotations: readOnly,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				list, err := s.Core.Cards(ctx, p, id, a.Board)
				return map[string]any{"cards": list}, err
			},
		},
		mcpTool{
			Name: "add_card", Title: "Add a board card",
			Description: "Put a card on a board. It goes last in its lane (the first lane when lane is left out) unless index says otherwise. Text is Markdown.",
			InputSchema: schema([]string{"board", "title"}, map[string]any{"account": accountProp, "board": boardProp, "lane": str("Lane name; defaults to the first lane."), "index": map[string]any{"type": "integer", "minimum": 0, "description": "Place in the lane, from 0."}, "title": str("Card title."), "text": str("Details, in Markdown."), "subject": subjectProp}),
			Annotations: writes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.AddCard(ctx, p, id, core.CardInput{Board: a.Board, Lane: a.Lane, Index: a.Index, Title: a.Title, Text: a.Text, Subject: a.Subject})
			},
		},
		mcpTool{
			Name: "update_card", Title: "Update a board card",
			Description: "Change a card, or move it: lane moves it to another lane (last, unless index is given), index moves it within its lane. Leave out what stays the same.",
			InputSchema: schema([]string{"id"}, map[string]any{"account": accountProp, "id": str("Card id from list_cards."), "lane": str("Lane name on the card's board."), "index": map[string]any{"type": "integer", "minimum": 0, "description": "Place in the lane, from 0."}, "title": str("Card title."), "text": str("Details, in Markdown."), "subject": subjectProp}),
			Annotations: writes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.UpdateCard(ctx, p, id, a.ID, core.CardInput{Lane: a.Lane, Index: a.Index, Title: a.Title, Text: a.Text, Subject: a.Subject})
			},
		},
		mcpTool{
			Name: "delete_card", Title: "Delete a board card",
			Description: "Delete a card from a board. This cannot be undone.",
			InputSchema: schema([]string{"id"}, map[string]any{"account": accountProp, "id": str("Card id from list_cards.")}),
			Annotations: deletes,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": a.ID, "deleted": true}, s.Core.DeleteCard(ctx, p, id, a.ID)
			},
		},
	)
}
