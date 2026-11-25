package lab

import (
	"context"
	"errors"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Comparing team tools between two states of the active pair: a version and
// the working state ("Euer Stand jetzt", version 0), or two versions. The
// older state is always on the left. Reads work in a closed task.

// Now is the working state in a comparison.
const Now = 0

// ErrUnknownVersion: the pair has no such version.
var ErrUnknownVersion = errors.New("unknown version")

// ToolChanges are the team tools that differ between two states.
type ToolChanges struct {
	Left  int                `json:"left"`  // older version; 0 never, the working state is the newest
	Right int                `json:"right"` // newer version, 0 for the working state
	Tools []store.ToolChange `json:"tools"`
}

// ToolSide is one side of a tool comparison: the version and the tool as it
// was there, nil if the tool did not exist in it.
type ToolSide struct {
	Version int            `json:"version"`
	Tool    *store.Program `json:"tool"`
}

// ToolDiff is a team tool in two states with the changes between them.
type ToolDiff struct {
	Left    ToolSide          `json:"left"`
	Right   ToolSide          `json:"right"`
	Changes []programs.Change `json:"changes"`
}

// toolsAt loads the team tools of a version, or of the working state.
func (s *Service) toolsAt(ctx context.Context, tt store.TeamTask, version int) ([]store.Program, error) {
	if version == Now {
		return s.Store.Programs(ctx, tt)
	}
	p, err := s.Store.GetPrompt(ctx, tt, version)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnknownVersion
	}
	if err != nil {
		return nil, err
	}
	return s.Store.VersionPrograms(ctx, p.ID)
}

// sides orders two states, older first, and loads their tools.
func (s *Service) sides(ctx context.Context, teamID string, version, against int) (int, int, []store.Program, []store.Program, error) {
	tt, _, err := s.ActiveTask(ctx, teamID)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if version < 1 || against < 0 {
		return 0, 0, nil, nil, ErrUnknownVersion
	}
	left, right := version, against
	if against != Now && against < version {
		left, right = against, version
	}
	l, err := s.toolsAt(ctx, tt, left)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	r, err := s.toolsAt(ctx, tt, right)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	return left, right, l, r, nil
}

// CompareTools lists the team tools that differ between a version and
// another one (against), or the working state (against Now).
func (s *Service) CompareTools(ctx context.Context, teamID string, version, against int) (ToolChanges, error) {
	left, right, l, r, err := s.sides(ctx, teamID, version, against)
	if err != nil {
		return ToolChanges{}, err
	}
	tools := store.DiffTools(l, r)
	if tools == nil {
		tools = []store.ToolChange{}
	}
	return ToolChanges{Left: left, Right: right, Tools: tools}, nil
}

// CompareTool compares one team tool between the two states. A tool in
// neither is ErrUnknownProgram.
func (s *Service) CompareTool(ctx context.Context, teamID string, version, against int, id string) (ToolDiff, error) {
	left, right, l, r, err := s.sides(ctx, teamID, version, against)
	if err != nil {
		return ToolDiff{}, err
	}
	d := ToolDiff{Left: ToolSide{Version: left, Tool: pick(l, id)}, Right: ToolSide{Version: right, Tool: pick(r, id)}}
	if d.Left.Tool == nil && d.Right.Tool == nil {
		return ToolDiff{}, ErrUnknownProgram
	}
	side := func(p *store.Program) programs.Side {
		if p == nil {
			return programs.Side{}
		}
		return programs.Side{Name: p.Name, Description: p.Description, Blockly: p.Blockly}
	}
	d.Changes, err = programs.Diff(side(d.Left.Tool), side(d.Right.Tool))
	if err != nil {
		return ToolDiff{}, err
	}
	if d.Changes == nil {
		d.Changes = []programs.Change{}
	}
	return d, nil
}

func pick(list []store.Program, id string) *store.Program {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}
