package web

import (
	"net/http"
	"strconv"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// compareArgs reads the version from the path and ?against= (a version or
// "now", the default) from the query. ok is false after a 404.
func compareArgs(w http.ResponseWriter, r *http.Request) (version, against int, ok bool) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		writeError(w, http.StatusNotFound, "Diese Version gibt es nicht.")
		return 0, 0, false
	}
	switch a := r.URL.Query().Get("against"); a {
	case "", "now":
		return v, lab.Now, true
	default:
		n, err := strconv.Atoi(a)
		if err != nil || n < 1 {
			writeError(w, http.StatusNotFound, "Diese Version gibt es nicht.")
			return 0, 0, false
		}
		return v, n, true
	}
}

// compareTools lists the team tools that differ between a version and the
// working state or another version.
func (s *Server) compareTools(w http.ResponseWriter, r *http.Request, team store.Team) {
	v, against, ok := compareArgs(w, r)
	if !ok {
		return
	}
	out, err := s.Lab.CompareTools(r.Context(), team.ID, v, against)
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// compareTool answers one team tool in both states with the changes.
func (s *Server) compareTool(w http.ResponseWriter, r *http.Request, team store.Team) {
	v, against, ok := compareArgs(w, r)
	if !ok {
		return
	}
	out, err := s.Lab.CompareTool(r.Context(), team.ID, v, against, r.PathValue("id"))
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
