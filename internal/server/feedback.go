package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// Feedback: suggestions and bug reports from the button in the app. Each one
// is a thread the user and the admins can write back and forth in.

const (
	maxFeedbackText = 4000
	maxOpenFeedback = 10
)

type feedbackView struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Page      string    `json:"page"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Preview   string    `json:"preview"`
	Unread    bool      `json:"unread"`
	Messages  int       `json:"messages"`
	// Admin views only: whose it is. AccountName is the linked account that
	// was open when it was sent (a parent's child, say).
	ProfileID   string `json:"profileId,omitempty"`
	ProfileName string `json:"profileName,omitempty"`
	AccountName string `json:"accountName,omitempty"`
}

type feedbackMessageView struct {
	ID        int64     `json:"id"`
	FromAdmin bool      `json:"fromAdmin"`
	Body      string    `json:"body"`
	At        time.Time `json:"at"`
}

func toFeedbackView(f store.Feedback) feedbackView {
	preview := f.Preview
	if utf8.RuneCountInString(preview) > 140 {
		preview = string([]rune(preview)[:140]) + "…"
	}
	return feedbackView{
		ID: f.ID, Kind: f.Kind, Status: f.Status, Page: f.Page, Preview: preview, Unread: f.Unread, Messages: f.Messages,
		CreatedAt: time.Unix(f.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(f.UpdatedAt, 0).UTC(),
	}
}

func toMessageViews(msgs []store.FeedbackMessage) []feedbackMessageView {
	out := []feedbackMessageView{}
	for _, m := range msgs {
		// Which admin answered stays private; the user hears from "Melon".
		out = append(out, feedbackMessageView{ID: m.ID, FromAdmin: m.FromAdmin, Body: m.Body, At: time.Unix(m.At, 0).UTC()})
	}
	return out
}

// feedbackText reads and checks the text of a new message.
func feedbackText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxFeedbackText {
		return "", core.Errf(http.StatusBadRequest, "invalid_request", "A message needs some text, at most %d characters.", maxFeedbackText)
	}
	return text, nil
}

// notWhileViewingAs keeps an admin who is looking at Melon as someone else
// from writing feedback in that person's name.
func (s *Server) notWhileViewingAs(w http.ResponseWriter, r *http.Request) bool {
	if _, by := s.sessionInfo(r); by != "" {
		writeErr(w, core.Errf(http.StatusForbidden, "impersonating", "Stop viewing as someone else first."))
		return false
	}
	return true
}

func (s *Server) feedbackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/app/feedback", s.sessionOnly(s.handleMyFeedback))
	mux.HandleFunc("POST /api/app/feedback", s.sessionOnly(s.handleCreateFeedback))
	mux.HandleFunc("GET /api/app/feedback/{id}", s.sessionOnly(s.handleMyFeedbackThread))
	mux.HandleFunc("POST /api/app/feedback/{id}/messages", s.sessionOnly(s.handleMyFeedbackReply))
	mux.HandleFunc("GET /api/app/admin/feedback", s.adminOnly(s.handleAdminFeedback))
	mux.HandleFunc("GET /api/app/admin/feedback/{id}", s.adminOnly(s.handleAdminFeedbackThread))
	mux.HandleFunc("POST /api/app/admin/feedback/{id}/messages", s.adminOnly(s.handleAdminFeedbackReply))
	mux.HandleFunc("PUT /api/app/admin/feedback/{id}", s.adminOnly(s.handleAdminFeedbackStatus))
}

// ---- the user's side ----

func (s *Server) handleMyFeedback(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, err := s.Store.UserFeedback(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []feedbackView{}
	for _, f := range list {
		out = append(out, toFeedbackView(f))
	}
	writeData(w, out)
}

func (s *Server) handleCreateFeedback(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if !s.notWhileViewingAs(w, r) {
		return
	}
	var body struct {
		Kind    string `json:"kind"`
		Text    string `json:"text"`
		Page    string `json:"page"`
		Account string `json:"account"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Kind != "idea" && body.Kind != "bug" {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "kind is idea or bug."))
		return
	}
	text, err := feedbackText(body.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	page := strings.TrimSpace(body.Page)
	if len(page) > 200 {
		page = page[:200]
	}
	if n, err := s.Store.OpenFeedbackCount(r.Context(), p.UserID); err != nil {
		writeErr(w, err)
		return
	} else if n >= maxOpenFeedback {
		writeErr(w, core.Errf(http.StatusTooManyRequests, "too_much_feedback", "You have %d open messages already. Add to one of those instead.", n))
		return
	}
	// Only a name from the profile's own accounts, never whatever was sent.
	accountName := ""
	if body.Account != "" {
		if accounts, err := s.Store.Accounts(r.Context(), p.UserID); err == nil {
			for _, a := range accounts {
				if a.ID == body.Account {
					accountName = a.DisplayName
				}
			}
		}
	}
	id := secret.ID("fb")
	if err := s.Store.CreateFeedback(r.Context(), id, p.UserID, body.Kind, page, accountName, text); err != nil {
		writeErr(w, err)
		return
	}
	s.writeThread(w, r, id, p.UserID, false, http.StatusCreated)
}

func (s *Server) handleMyFeedbackThread(w http.ResponseWriter, r *http.Request, p core.Principal) {
	s.writeThread(w, r, r.PathValue("id"), p.UserID, false, http.StatusOK)
}

func (s *Server) handleMyFeedbackReply(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if !s.notWhileViewingAs(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, _, err := s.Store.FeedbackThread(r.Context(), id, p.UserID, false); err != nil {
		writeErr(w, notFound(err))
		return
	}
	if !s.reply(w, r, id, p.UserID, false) {
		return
	}
	s.writeThread(w, r, id, p.UserID, false, http.StatusOK)
}

// ---- the admins' side ----

func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	status := r.URL.Query().Get("status")
	if status != "open" && status != "closed" {
		status = ""
	}
	list, err := s.Store.AllFeedback(r.Context(), status, 200)
	if err != nil {
		writeErr(w, err)
		return
	}
	names := map[string]string{}
	out := []feedbackView{}
	for _, f := range list {
		v := toFeedbackView(f)
		v.ProfileID = f.UserID
		if _, ok := names[f.UserID]; !ok {
			names[f.UserID] = s.profileName(r.Context(), f.UserID)
		}
		v.ProfileName = names[f.UserID]
		v.AccountName = f.AccountName
		out = append(out, v)
	}
	writeData(w, out)
}

func (s *Server) handleAdminFeedbackThread(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	s.writeThread(w, r, r.PathValue("id"), "", true, http.StatusOK)
}

func (s *Server) handleAdminFeedbackReply(w http.ResponseWriter, r *http.Request, p core.Principal) {
	id := r.PathValue("id")
	if !s.reply(w, r, id, p.UserID, true) {
		return
	}
	s.audit(r.Context(), p, "feedback_reply", id, "")
	s.writeThread(w, r, id, "", true, http.StatusOK)
}

func (s *Server) handleAdminFeedbackStatus(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Status != "open" && body.Status != "closed" {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "status is open or closed."))
		return
	}
	id := r.PathValue("id")
	if err := s.Store.SetFeedbackStatus(r.Context(), id, body.Status); err != nil {
		writeErr(w, notFound(err))
		return
	}
	s.audit(r.Context(), p, "feedback_"+body.Status, id, "")
	s.writeThread(w, r, id, "", true, http.StatusOK)
}

// ---- shared ----

func (s *Server) reply(w http.ResponseWriter, r *http.Request, id, author string, fromAdmin bool) bool {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return false
	}
	text, err := feedbackText(body.Text)
	if err != nil {
		writeErr(w, err)
		return false
	}
	if err := s.Store.AddFeedbackMessage(r.Context(), id, author, fromAdmin, text); err != nil {
		writeErr(w, notFound(err))
		return false
	}
	return true
}

// writeThread answers with one thread and its messages, and marks it seen.
// userID limits it to that profile's own threads; admins pass "".
func (s *Server) writeThread(w http.ResponseWriter, r *http.Request, id, userID string, asAdmin bool, status int) {
	f, msgs, err := s.Store.FeedbackThread(r.Context(), id, userID, asAdmin)
	if err != nil {
		writeErr(w, notFound(err))
		return
	}
	v := toFeedbackView(f)
	v.Unread = false // just read
	if asAdmin {
		v.ProfileID = f.UserID
		v.ProfileName = s.profileName(r.Context(), f.UserID)
		v.AccountName = f.AccountName
	}
	writeJSON(w, status, map[string]any{"data": map[string]any{"thread": v, "messages": toMessageViews(msgs)}})
}

// profileName is who is behind a profile: the parent's own name when the
// accounts are a parent's children, not the first child's.
func (s *Server) profileName(ctx context.Context, userID string) string {
	accounts, err := s.Store.Accounts(ctx, userID)
	if err != nil || len(accounts) == 0 {
		return ""
	}
	for _, a := range accounts {
		if a.OwnerCode == "" {
			return a.DisplayName
		}
	}
	if accounts[0].OwnerName != "" {
		return accounts[0].OwnerName
	}
	return accounts[0].DisplayName
}

func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return core.ErrNotFound
	}
	return err
}
