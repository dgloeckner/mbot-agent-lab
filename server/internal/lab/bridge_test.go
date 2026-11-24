package lab

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func TestJoinActivatesTheDefaultTask(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newService(t, st, nil)
	for _, name := range []string{"Rot", "Blau"} {
		team, err := s.Join(ctx, "TEST", name)
		if err != nil {
			t.Fatal(err)
		}
		tt, err := st.ActiveTask(ctx, team.ID)
		if err != nil || tt.Task != DefaultTaskID {
			t.Fatalf("%s: active task %+v, %v", name, tt, err)
		}
		var got []string
		evs, _ := st.Events(ctx, "")
		for _, e := range evs {
			if e.Team == team.ID {
				got = append(got, e.Kind+"/"+e.Task+"/"+e.Actor)
			}
		}
		want := []string{"team.joined//team", "task.assigned/default/system", "task.accepted/default/system", "task.activated/default/system"}
		if !slices.Equal(got, want) {
			t.Errorf("%s: events %v, want %v", name, got, want)
		}
		// The team can work in it right away.
		if _, err := st.SavePrompt(ctx, tt, "Los."); err != nil {
			t.Errorf("%s: prompt in the default task: %v", name, err)
		}
	}
}
