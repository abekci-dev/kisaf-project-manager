package server

import (
	"errors"
	"net/http"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
	"github.com/abekci-dev/kisaf-project-manager/internal/store"
)

// Every handler here answers with the whole project rather than just the task
// that changed. A task list is small, and returning the full row lets the UI
// replace its copy in one assignment instead of reconciling a nested list —
// which is where "the checkbox ticked but the counter did not" bugs come from.

type addTodoRequest struct {
	Text     string         `json:"text"`
	Priority store.Priority `json:"priority"`
}

func (s *Server) handleAddTodo(w http.ResponseWriter, r *http.Request) {
	var in addTodoRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	project, err := s.store.AddTodo(r.PathValue("id"), in.Text, in.Priority)
	if err != nil {
		s.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": project})
}

func (s *Server) handleUpdateTodo(w http.ResponseWriter, r *http.Request) {
	var patch store.TodoPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	project, err := s.store.UpdateTodo(r.PathValue("id"), r.PathValue("todoId"), patch)
	if err != nil {
		s.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) handleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.DeleteTodo(r.PathValue("id"), r.PathValue("todoId"))
	if err != nil {
		s.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) handleClearDoneTodos(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.ClearDoneTodos(r.PathValue("id"))
	if err != nil {
		s.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

type reorderRequest struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleReorderTodos(w http.ResponseWriter, r *http.Request) {
	var in reorderRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	project, err := s.store.ReorderTodos(r.PathValue("id"), in.IDs)
	if err != nil {
		s.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

// writeTodoError maps a store error onto a status code. A missing task is a
// 404 like a missing project, but everything else the store rejects here is a
// bad request (empty text, task limit) rather than a server fault.
func (s *Server) writeTodoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrTodoNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
