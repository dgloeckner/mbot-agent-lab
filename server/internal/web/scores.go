package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
)

// The control room's "Läufe": runs of a task with their measures, the
// ranking, and the lab lead's confirmations and interventions.

func (s *Server) adminTaskRuns(w http.ResponseWriter, r *http.Request) {
	teams, err := s.Lab.TaskScores(r.Context(), r.PathValue("id"))
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"teams": teams})
}

func runID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Diesen Lauf gibt es nicht.")
		return 0, false
	}
	return id, true
}

// adminConfirmRun records whether a run reached the goal: {success}.
func (s *Server) adminConfirmRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	var req struct {
		Success *bool `json:"success"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Success == nil {
		writeError(w, http.StatusBadRequest, "Sagt, ob der Lauf geschafft ist: success true oder false.")
		return
	}
	if err := s.Lab.ConfirmRun(r.Context(), id, *req.Success); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminIntervention records an intervention the lab lead saw: {kind: "moved"}.
func (s *Server) adminIntervention(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.AddIntervention(r.Context(), id, req.Kind); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminStartRun starts a team's run from the control room.
func (s *Server) adminStartRun(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	if _, err := s.Store.Team(r.Context(), teamID); err != nil {
		writeError(w, http.StatusNotFound, "Dieses Team gibt es nicht mehr.")
		return
	}
	if err := s.Agents.Start(r.Context(), teamID); err != nil {
		var se *agent.StartError
		if errors.As(err, &se) {
			writeError(w, http.StatusConflict, se.Msg)
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.Agents.Status(teamID))
}
