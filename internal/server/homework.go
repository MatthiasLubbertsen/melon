package server

import (
	"context"
	"net/http"
	"strings"

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

func (s *Server) handleSetWeekPlan(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	u, err := s.Core.SetWeekPlan(r.Context(), p, r.PathValue("account"), r.PathValue("subject"), r.PathValue("week"), body.URL)
	if err != nil {
		writeErr(w, err)
		return
	}
	if u == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeData(w, map[string]string{"subject": strings.ToLower(r.PathValue("subject")), "week": strings.ToUpper(r.PathValue("week")), "url": u})
}

func (s *Server) handleDeleteWeekPlan(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if _, err := s.Core.SetWeekPlan(r.Context(), p, r.PathValue("account"), r.PathValue("subject"), r.PathValue("week"), ""); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func init() {
	mcpTools = append(mcpTools,
		mcpTool{
			Name: "list_homework", Title: "List homework",
			Description: "List the homework the user wrote on their lessons (Markdown), each with the lesson it is on as Melon last saw it. A lesson status of 'cancelled' or 'gone' means the lesson dropped out and the homework may need a new lesson. Homework also shows on each lesson in get_schedule, next to weekPlan (a link saved with set_week_plan).",
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
				return s.Core.AddHomework(ctx, p, id, a.Lesson, a.Date, deref(a.Text))
			},
		},
		mcpTool{
			Name: "update_homework", Title: "Update homework",
			Description: "Change a piece of homework: its text (Markdown), whether it is done, or the lesson it is on. move_to 'next' puts it on the next lesson of the same subject that still takes place (use it when its lesson dropped out); a lesson id or instance from get_schedule, with date, puts it on that lesson. Leave out what stays the same.",
			InputSchema: schema([]string{"id"}, map[string]any{
				"account": accountProp,
				"id":      str("Homework id from list_homework or get_schedule."),
				"text":    str("New text, in Markdown."),
				"done":    map[string]any{"type": "boolean", "description": "Whether the homework is done."},
				"move_to": str("'next', or a lesson id or instance."),
				"date":    str("The date of the lesson in move_to, YYYY-MM-DD."),
			}),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false},
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return s.Core.UpdateHomework(ctx, p, id, a.ID, core.HomeworkChange{Text: a.Text, Done: a.Done, Move: a.MoveTo, Date: a.Date})
			},
		},
		mcpTool{
			Name: "delete_homework", Title: "Delete homework",
			Description: "Delete a piece of homework. This cannot be undone.",
			InputSchema: schema([]string{"id"}, map[string]any{"account": accountProp, "id": str("Homework id from list_homework or get_schedule.")}),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": false},
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": a.ID, "deleted": true}, s.Core.DeleteHomework(ctx, p, id, a.ID)
			},
		},
		mcpTool{
			Name: "set_week_plan", Title: "Set a week plan link",
			Description: "Save a link (an itslearning week plan, say) for one subject in one ISO week. It shows as weekPlan on every lesson of that subject that week in get_schedule. Pass an empty url to remove it. Melon does not open the link.",
			InputSchema: schema([]string{"code", "week", "url"}, map[string]any{
				"account": accountProp,
				"code":    str("Subject code, e.g. 'du'."),
				"week":    str("ISO week, e.g. 2026-W48."),
				"url":     str("https:// link, or empty to remove."),
			}),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
			run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
				id, err := s.mcpAccount(ctx, p, a.Account)
				if err != nil {
					return nil, err
				}
				u, err := s.Core.SetWeekPlan(ctx, p, id, a.Code, a.Week, a.URL)
				return map[string]any{"code": strings.ToLower(a.Code), "week": strings.ToUpper(a.Week), "url": u}, err
			},
		},
	)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
