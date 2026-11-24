package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Team tools: the working state of a (team, task) pair. The browser sends
// name, description and Blockly state; the server derives tree and problems
// and stores the tool even when it is invalid. One writer per team: last
// write wins, no version check, no lock. Every save tells the team's other
// pages (SSE "programs").

// ErrUnknownProgram is returned for a team tool the active pair does not have.
var ErrUnknownProgram = errors.New("unknown team tool")

// ProgramInfo is a team tool in the list: whether it can run, and why not.
type ProgramInfo struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Valid       bool               `json:"valid"`
	Problems    []programs.Problem `json:"problems"`
	UpdatedAt   string             `json:"updated_at"`
}

// ProgramDetail is one team tool with its Blockly state.
type ProgramDetail struct {
	ProgramInfo
	Blockly json.RawMessage `json:"blockly"`
}

func info(p store.Program) ProgramInfo {
	var problems []programs.Problem
	_ = json.Unmarshal(p.Problems, &problems)
	if problems == nil {
		problems = []programs.Problem{}
	}
	return ProgramInfo{ID: p.ID, Name: p.Name, Description: p.Description, Valid: len(problems) == 0, Problems: problems,
		UpdatedAt: p.UpdatedAt.Format("2006-01-02T15:04:05.000Z07:00")}
}

// Programs lists the team tools of the team's active task.
func (s *Service) Programs(ctx context.Context, teamID string) ([]ProgramInfo, error) {
	tt, _, err := s.ActiveTask(ctx, teamID)
	if err != nil {
		return nil, err
	}
	list, err := s.Store.Programs(ctx, tt)
	if err != nil {
		return nil, err
	}
	out := make([]ProgramInfo, len(list))
	for i, p := range list {
		out[i] = info(p)
	}
	return out, nil
}

// Program returns one team tool of the active task with its Blockly state.
func (s *Service) Program(ctx context.Context, teamID, id string) (ProgramDetail, error) {
	tt, _, err := s.ActiveTask(ctx, teamID)
	if err != nil {
		return ProgramDetail{}, err
	}
	return s.program(ctx, tt, id)
}

func (s *Service) program(ctx context.Context, tt store.TeamTask, id string) (ProgramDetail, error) {
	p, err := s.Store.GetProgram(ctx, tt, id)
	if errors.Is(err, store.ErrNotFound) {
		return ProgramDetail{}, ErrUnknownProgram
	}
	if err != nil {
		return ProgramDetail{}, err
	}
	return ProgramDetail{ProgramInfo: info(p), Blockly: p.Blockly}, nil
}

// LoadProgram returns the tree of a team tool of the active, open task and
// its problems; the runner uses it.
func (s *Service) LoadProgram(ctx context.Context, teamID, id string) (store.TeamTask, *programs.Program, []programs.Problem, error) {
	tt, err := s.WritableTask(ctx, teamID)
	if err != nil {
		return tt, nil, nil, err
	}
	p, err := s.Store.GetProgram(ctx, tt, id)
	if errors.Is(err, store.ErrNotFound) {
		return tt, nil, nil, ErrUnknownProgram
	}
	if err != nil {
		return tt, nil, nil, err
	}
	tree, err := programs.Parse(p.Tree)
	if err != nil {
		return tt, nil, nil, err
	}
	return tt, tree, info(p).Problems, nil
}

// SaveProgram creates (id "") or replaces a team tool of the active, open
// task from its name, description and Blockly state. The tool is stored
// even when it is invalid; its problems come back per block.
func (s *Service) SaveProgram(ctx context.Context, teamID, id, name, description string, blockly json.RawMessage) (ProgramDetail, error) {
	tt, err := s.WritableTask(ctx, teamID)
	if err != nil {
		return ProgramDetail{}, err
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if len(blockly) == 0 || string(blockly) == "null" {
		blockly = json.RawMessage(`{}`)
	}
	if !json.Valid(blockly) {
		return ProgramDetail{}, userErr("Die Blöcke konnten nicht gelesen werden.")
	}
	create := id == ""
	if create {
		n, err := s.Store.CountPrograms(ctx, tt)
		if err != nil {
			return ProgramDetail{}, err
		}
		if n >= programs.MaxPerTask {
			return ProgramDetail{}, &UserError{Msg: fmt.Sprintf("Ihr habt schon %d eigene Werkzeuge in dieser Aufgabe. Löscht zuerst eins.", programs.MaxPerTask), Conflict: true}
		}
		id = "p" + randomString(8)
	}
	tree, problems := programs.Convert(name, description, blockly, s.Tools)
	if problems == nil {
		problems = []programs.Problem{}
	}
	treeJSON, _ := json.Marshal(tree)
	problemsJSON, _ := json.Marshal(problems)
	_, err = s.Store.SaveProgram(ctx, tt, store.Program{ID: id, Name: name, Description: description, Blockly: blockly, Tree: treeJSON, Problems: problemsJSON}, create)
	switch {
	case errors.Is(err, store.ErrConflict):
		return ProgramDetail{}, &UserError{Msg: fmt.Sprintf("Ein Werkzeug namens „%s“ habt ihr schon. Wählt einen anderen Namen.", name), Conflict: true}
	case errors.Is(err, store.ErrNotFound):
		return ProgramDetail{}, ErrUnknownProgram
	case err != nil:
		return ProgramDetail{}, err
	}
	s.Log.Info("team tool saved", "team", teamID, "task", tt.Task, "id", id, "name", name, "valid", len(problems) == 0, "problems", len(problems))
	s.Hub.SignalRef(teamID, live.SignalPrograms, id)
	return s.program(ctx, tt, id)
}

// DeleteProgram removes a team tool of the active, open task.
func (s *Service) DeleteProgram(ctx context.Context, teamID, id string) error {
	tt, err := s.WritableTask(ctx, teamID)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteProgram(ctx, tt, id); errors.Is(err, store.ErrNotFound) {
		return ErrUnknownProgram
	} else if err != nil {
		return err
	}
	s.Log.Info("team tool deleted", "team", teamID, "task", tt.Task, "id", id)
	s.Hub.SignalRef(teamID, live.SignalPrograms, id)
	return nil
}
