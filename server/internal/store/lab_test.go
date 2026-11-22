package store

import (
	"context"
	"errors"
	"testing"
)

func TestTeamsAndAssignments(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	rot, err := s.CreateTeam(ctx, Team{ID: "t1", Name: "Kurvenkratzer", Token: "tok1", Color: "red"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTeam(ctx, Team{ID: "t2", Name: "kurvenKRATZER", Token: "tok2", Color: "blue"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("names must be unique ignoring case, got %v", err)
	}
	blau, _ := s.CreateTeam(ctx, Team{ID: "t3", Name: "Blau", Token: "tok3", Color: "blue"})

	got, err := s.TeamByToken(ctx, "tok1")
	if err != nil || got.ID != rot.ID {
		t.Fatalf("TeamByToken = %+v, %v", got, err)
	}
	if _, err := s.TeamByToken(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token: %v", err)
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Assign(ctx, "mbot-1", rot.ID, "Scout"))
	must(s.Assign(ctx, "mbot-2", rot.ID, "Träger"))
	if err := s.Assign(ctx, "mbot-3", rot.ID, "scout"); !errors.Is(err, ErrConflict) {
		t.Errorf("alias must be unique per team: %v", err)
	}
	must(s.Assign(ctx, "mbot-3", blau.ID, "Scout")) // same alias in another team is fine
	must(s.Assign(ctx, "mbot-2", blau.ID, "Zwei"))  // reassignment replaces the owner

	as, _ := s.Assignments(ctx)
	owners := map[string]string{}
	for _, a := range as {
		owners[a.RobotID] = a.TeamID + "/" + a.Alias
	}
	want := map[string]string{"mbot-1": "t1/Scout", "mbot-2": "t3/Zwei", "mbot-3": "t3/Scout"}
	for k, v := range want {
		if owners[k] != v {
			t.Errorf("%s owned by %q, want %q", k, owners[k], v)
		}
	}

	must(s.DeleteTeam(ctx, blau.ID))
	as, _ = s.Assignments(ctx)
	if len(as) != 1 || as[0].RobotID != "mbot-1" {
		t.Errorf("deleting a team must free its robots: %+v", as)
	}
	must(s.Assign(ctx, "mbot-1", "", ""))
	if as, _ = s.Assignments(ctx); len(as) != 0 {
		t.Errorf("free: %+v", as)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if v, err := s.Setting(ctx, "lab_code"); err != nil || v != "" {
		t.Fatalf("unset = %q, %v", v, err)
	}
	_ = s.SetSetting(ctx, "lab_code", "K7QX")
	_ = s.SetSetting(ctx, "lab_code", "AB12")
	if v, _ := s.Setting(ctx, "lab_code"); v != "AB12" {
		t.Errorf("lab_code = %q", v)
	}
}
