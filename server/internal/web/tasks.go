package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func (s *Server) taskAdminRoutes(admin func(string, http.HandlerFunc)) {
	admin("GET /admin/api/tasks", s.adminTasks)
	admin("POST /admin/api/tasks", s.adminCreateTask)
	admin("GET /admin/api/tasks/{id}", s.adminTask)
	admin("PUT /admin/api/tasks/{id}", s.adminUpdateTask)
	admin("DELETE /admin/api/tasks/{id}", s.adminDeleteTask)
	admin("POST /admin/api/tasks/{id}/publish", s.adminPublishTask)
	admin("POST /admin/api/tasks/{id}/close", s.adminCloseTask)
	admin("POST /admin/api/tasks/{id}/duplicate", s.adminDuplicateTask)
	admin("POST /admin/api/tasks/{id}/assign", s.adminAssignTask)
	admin("DELETE /admin/api/tasks/{id}/assign/{team}", s.adminWithdrawTask)
}

// AdminTask is a task in the board payload, without its text.
type AdminTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at,omitzero"`
	ClosedAt    time.Time `json:"closed_at,omitzero"`
}

// AdminTeamTask is a task on a team card: where the team stands with it.
type AdminTeamTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"`  // draft | open | closed
	Status string `json:"status"` // assigned | accepted | active
}

func adminTask(t store.Task) AdminTask {
	return AdminTask{ID: t.ID, Title: t.Title, State: t.State, CreatedAt: t.CreatedAt, PublishedAt: t.PublishedAt, ClosedAt: t.ClosedAt}
}

func (s *Server) taskError(w http.ResponseWriter, err error) {
	if errors.Is(err, lab.ErrUnknownTask) {
		writeError(w, http.StatusNotFound, "Diese Aufgabe gibt es nicht.")
		return
	}
	s.labError(w, err)
}

func (s *Server) adminTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.Lab.Tasks(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) adminTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Lab.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) adminCreateTask(w http.ResponseWriter, r *http.Request) {
	var in lab.TaskInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.Lab.CreateTask(r.Context(), in)
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) adminUpdateTask(w http.ResponseWriter, r *http.Request) {
	var in lab.TaskInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.Lab.UpdateTask(r.Context(), r.PathValue("id"), in)
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) adminDeleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.Lab.DeleteTask(r.Context(), r.PathValue("id")); err != nil {
		s.taskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminPublishTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Lab.PublishTask(r.Context(), r.PathValue("id"))
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) adminCloseTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Lab.CloseTask(r.Context(), r.PathValue("id"))
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) adminDuplicateTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Lab.DuplicateTask(r.Context(), r.PathValue("id"))
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// adminAssignTask assigns a task: {"teams": ["t1", "t2"]} or {"teams": "all"}.
func (s *Server) adminAssignTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Teams json.RawMessage `json:"teams"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var teams []string
	var all string
	if err := json.Unmarshal(req.Teams, &all); err == nil {
		if all != lab.AllTeams {
			writeError(w, http.StatusBadRequest, "Wählt Teams oder „alle“.")
			return
		}
		teams = []string{lab.AllTeams}
	} else if err := json.Unmarshal(req.Teams, &teams); err != nil || len(teams) == 0 {
		writeError(w, http.StatusBadRequest, "Wählt Teams oder „alle“.")
		return
	}
	got, err := s.Lab.AssignTask(r.Context(), r.PathValue("id"), teams)
	if err != nil {
		s.taskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"assigned": got})
}

func (s *Server) adminWithdrawTask(w http.ResponseWriter, r *http.Request) {
	if err := s.Lab.WithdrawTask(r.Context(), r.PathValue("id"), r.PathValue("team")); err != nil {
		s.taskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
