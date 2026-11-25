package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Team tools of the active task. The rules live in lab (programs.go); the
// browser sends the Blockly state and never a tree.

func (s *Server) listPrograms(w http.ResponseWriter, r *http.Request, team store.Team) {
	list, err := s.Lab.Programs(r.Context(), team.ID)
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getProgram(w http.ResponseWriter, r *http.Request, team store.Team) {
	p, err := s.Lab.Program(r.Context(), team.ID, r.PathValue("id"))
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// saveProgram creates (POST) or replaces (PUT, last write wins) a team tool.
// It answers the stored tool with its problems per block, also when invalid.
func (s *Server) saveProgram(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Blockly     json.RawMessage `json:"blockly"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Die Anfrage war ungültig.")
		return
	}
	id := r.PathValue("id")
	p, err := s.Lab.SaveProgram(r.Context(), team.ID, id, req.Name, req.Description, req.Blockly)
	if err != nil {
		s.labError(w, err)
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, p)
}

func (s *Server) deleteProgram(w http.ResponseWriter, r *http.Request, team store.Team) {
	if err := s.Lab.DeleteProgram(r.Context(), team.ID, r.PathValue("id")); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// runProgram starts a team tool ("Ausführen"): {robot?, step?}. It answers
// the run id; the steps arrive as SSE "program" events. 409 during a run,
// for an invalid tool and while another team tool runs.
func (s *Server) runProgram(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Robot string `json:"robot"`
		Step  bool   `json:"step"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "Die Anfrage war ungültig.")
		return
	}
	run, err := s.Agents.RunProgram(r.Context(), team.ID, r.PathValue("id"), req.Robot, req.Step)
	if err != nil {
		s.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run": run})
}

// continueProgram lets a single-step run take its next call ("Weiter").
func (s *Server) continueProgram(w http.ResponseWriter, r *http.Request, team store.Team) {
	if err := s.Agents.ContinueProgram(team.ID, r.PathValue("id")); err != nil {
		s.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// agentError answers a refusal of the agent runtime with 409 and other
// errors like the lab's.
func (s *Server) agentError(w http.ResponseWriter, err error) {
	var se *agent.StartError
	if errors.As(err, &se) {
		writeError(w, http.StatusConflict, se.Msg)
		return
	}
	s.labError(w, err)
}
