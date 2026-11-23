package lab

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

type nopBus struct{}

func (nopBus) Publish(string, []byte, bool, byte) error       { return nil }
func (nopBus) Handle(string, int, func(string, []byte)) error { return nil }

func newService(t *testing.T, st *store.Store, providers map[string]config.Provider) *Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Service{
		Store: st, Robots: robots.New(nopBus{}, nil, log), Hub: live.NewHub(), Log: log,
		Config: &config.Config{Server: config.Server{LabCode: "TEST"}, LLM: config.LLM{Default: "mistral", Providers: providers}},
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestModelChoice(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	providers := map[string]config.Provider{
		"mistral": {Model: "mistral-small", PriceIn: 0.1, PriceOut: 0.3},
		"claude":  {Model: "claude-haiku-5-5", PriceIn: 0.1, PriceOut: 0.5},
		"opus":    {Model: "claude-opus-5-5", PriceIn: 4, PriceOut: 20},
	}
	s := newService(t, st, providers)
	team, _ := s.Join(ctx, "TEST", "Rot")

	ms := s.Models()
	if len(ms) != 3 || ms[0].Provider != "mistral" || !ms[0].Default || !ms[0].Allowed || ms[1].Allowed {
		t.Fatalf("models = %+v", ms)
	}
	if err := s.SetTeamModel(ctx, team.ID, "claude"); err == nil {
		t.Fatal("unreleased provider accepted")
	}
	gen := s.ModelsVersion()
	if err := s.SetAllowedModels(ctx, []string{"claude", "mistral"}); err != nil {
		t.Fatal(err)
	}
	if s.ModelsVersion() == gen {
		t.Error("version did not change")
	}
	if err := s.SetTeamModel(ctx, team.ID, "claude"); err != nil {
		t.Fatal(err)
	}
	if p := s.TeamProvider(ctx, team.ID); p != "claude" {
		t.Errorf("provider = %q", p)
	}
	if tm := s.ModelsFor(mustTeam(t, st, team.ID)); tm.Selected != "claude" || len(tm.Options) != 2 {
		t.Errorf("team models = %+v", tm)
	}

	// The release survives a restart; a provider gone from the config falls back.
	s2 := newService(t, st, providers)
	if p := s2.TeamProvider(ctx, team.ID); p != "claude" {
		t.Errorf("after restart = %q", p)
	}
	delete(providers, "claude")
	s3 := newService(t, st, providers)
	if p := s3.TeamProvider(ctx, team.ID); p != "mistral" {
		t.Errorf("unconfigured provider = %q", p)
	}

	// Choosing the default stores no choice, so the team follows the lab default.
	if err := s2.SetTeamModel(ctx, team.ID, "mistral"); err != nil {
		t.Fatal(err)
	}
	if got := mustTeam(t, st, team.ID); got.Provider != "" {
		t.Errorf("stored default choice = %q", got.Provider)
	}
}

func mustTeam(t *testing.T, st *store.Store, id string) store.Team {
	t.Helper()
	team, err := st.Team(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return team
}

func TestConversationSwitch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newService(t, st, map[string]config.Provider{"mistral": {Model: "m"}})
	team, _ := s.Join(ctx, "TEST", "Rot")
	if !s.ConversationEnabled() {
		t.Fatal("conversations must be on by default")
	}
	if err := s.SetConversationEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if s.ConversationEnabled() {
		t.Error("still on")
	}
	backlog, _, cancel := s.Hub.Subscribe(team.ID, 0)
	cancel()
	if len(backlog) == 0 || backlog[len(backlog)-1].Channel != live.ChannelConversation || !strings.Contains(backlog[len(backlog)-1].Text, "abgeschaltet") {
		t.Errorf("team not told: %+v", backlog)
	}
	// The switch survives a restart.
	if again := newService(t, st, map[string]config.Provider{"mistral": {Model: "m"}}); again.ConversationEnabled() {
		t.Error("switch not stored")
	}
}
