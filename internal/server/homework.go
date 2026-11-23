package server

import (
	"context"
	"net/http"

	"github.com/matthiaslubbertsen/melon/internal/core"
)

func (s *Server) handleHomeworkList(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, err := s.Core.HomeworkList(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, list)
}

func (s *Server) handleAddHomework(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Lesson string `json:"lesson"`
		Date   string `json:"date"`
		Text   string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	h, err := s.Core.AddHomework(r.Context(), p, r.PathValue("account"), body.Lesson, body.Date, body.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": h})
}

func (s *Server) handleUpdateHomework(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Text   *string `json:"text"`
		Done   *bool   `json:"done"`
		MoveTo string  `json:"moveTo"`
		Date   string  `json:"date"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	h, err := s.Core.UpdateHomework(r.Context(), p, r.PathValue("account"), r.PathValue("id"), core.HomeworkChange{Text: body.Text, Done: body.Done, Move: body.MoveTo, Date: body.Date})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, h)
}

func (s *Server) handleDeleteHomework(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.DeleteHomework(r.Context(), p, r.PathValue("account"), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func init() {
	mcpTools = append(mcpTools,
		mcpTool{
			Name: "list_homework", Title: "List homework",
			Description: "List the homework the user wrote on their lessons (Markdown), each with the lesson it is on as Melon last saw it. A lesson status of 'cancelled' or 'gone' means the lesson dropped out and the homework may need a new lesson. Homework also shows on each lesson in get_schedule.",
			InputSchema: schema(nil, map[string]any{"account": accountProp}), Annotations: readOnly,
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				list, err := s.Core.HomeworkList(ctx, p, id)
				return map[string]any{"homework": list}, err
			},
		},
		mcpTool{
			Name: "add_homework", Title: "Add homework",
			Description: "Write homework on one of the user's own lessons. It stays on that lesson when Zermelo moves it. Text is Markdown. Only stored in Melon; nothing is written to Zermelo.",
			InputSchema: schema([]string{"lesson", "date", "text"}, map[string]any{
				"account": accountProp,
				"lesson":  str("The lesson's id or instance, from get_schedule."),
				"date":    str("The lesson's date, YYYY-MM-DD."),
				"text":    str("The homework, in Markdown."),
			}),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false},
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.AddHomework(ctx, p, id, a.Lesson, a.Date, a.Text)
			},
		},
	)
}
