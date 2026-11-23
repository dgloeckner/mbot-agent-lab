package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Model choice: every team runs on the lab's default provider unless it picks
// another one on its team page. Teams may only pick providers the control room
// released; the default provider is always allowed. A change applies at the
// team's next run, so a run keeps one model from start to end.

// teamModelsKey stores the released providers (besides the default) as a JSON list.
const teamModelsKey = "team_models"

// Model is one configured provider as a choice for teams.
type Model struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	PriceIn  float64 `json:"price_in"`  // per million input tokens, as configured
	PriceOut float64 `json:"price_out"` // per million output tokens
	Default  bool    `json:"default"`
	Allowed  bool    `json:"allowed"` // teams may choose it
}

// TeamModels is what a team page shows: its current choice and its options.
type TeamModels struct {
	Selected string  `json:"selected"` // provider the team's next run uses
	Options  []Model `json:"options"`  // providers the team may choose, default first
}

// loadAllowed reads the released providers. Call with s.mu held or before use.
func (s *Service) loadAllowed(ctx context.Context) error {
	raw, err := s.Store.Setting(ctx, teamModelsKey)
	if err != nil {
		return err
	}
	var names []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &names); err != nil {
			s.Log.Warn("ignoring stored team models", "err", err)
		}
	}
	s.allowed = map[string]bool{}
	for _, n := range names {
		s.allowed[n] = true
	}
	return nil
}

func (s *Service) defaultProvider() string { return s.Config.LLM.Default }

func (s *Service) configured(provider string) bool {
	_, ok := s.Config.LLM.Providers[provider]
	return ok
}

// allowedLocked reports whether teams may use provider. Call with s.mu held.
func (s *Service) allowedLocked(provider string) bool {
	return s.configured(provider) && (provider == s.defaultProvider() || s.allowed[provider])
}

// Models lists every configured provider, the default first, then by name.
func (s *Service) Models() []Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelsLocked()
}

func (s *Service) modelsLocked() []Model {
	out := make([]Model, 0, len(s.Config.LLM.Providers))
	for name, p := range s.Config.LLM.Providers {
		out = append(out, Model{
			Provider: name, Model: p.Model, PriceIn: p.PriceIn, PriceOut: p.PriceOut,
			Default: name == s.defaultProvider(), Allowed: s.allowedLocked(name),
		})
	}
	slices.SortFunc(out, func(a, b Model) int {
		if a.Default != b.Default {
			if a.Default {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Provider, b.Provider)
	})
	return out
}

// Provider returns the provider a team's next run uses: its own choice if
// that is still configured and released, else the lab default.
func (s *Service) Provider(t store.Team) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.providerLocked(t)
}

func (s *Service) providerLocked(t store.Team) string {
	if t.Provider != "" && s.allowedLocked(t.Provider) {
		return t.Provider
	}
	return s.defaultProvider()
}

// TeamProvider is Provider by team ID; an unknown team gets the default.
func (s *Service) TeamProvider(ctx context.Context, teamID string) string {
	t, err := s.Store.Team(ctx, teamID)
	if err != nil {
		return s.defaultProvider()
	}
	return s.Provider(t)
}

// ModelsFor returns the choice and options a team page shows.
func (s *Service) ModelsFor(t store.Team) TeamModels {
	s.mu.Lock()
	defer s.mu.Unlock()
	tm := TeamModels{Selected: s.providerLocked(t), Options: []Model{}}
	for _, m := range s.modelsLocked() {
		if m.Allowed {
			tm.Options = append(tm.Options, m)
		}
	}
	return tm
}

// ModelsVersion changes whenever a team's choice or the released providers
// change, so live views know when to look again.
func (s *Service) ModelsVersion() int64 { return s.modelsGen.Load() }

// SetTeamModel records the provider a team chose for its next runs.
func (s *Service) SetTeamModel(ctx context.Context, teamID, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.Store.Team(ctx, teamID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrUnknownTeam
	} else if err != nil {
		return err
	}
	if !s.allowedLocked(provider) {
		return userErr("Dieses Modell könnt ihr gerade nicht wählen. Ladet die Seite neu.")
	}
	stored := provider
	if provider == s.defaultProvider() {
		stored = "" // follow the lab default
	}
	if t.Provider == stored {
		return nil
	}
	if err := s.Store.SetTeamProvider(ctx, teamID, stored); err != nil {
		return err
	}
	s.modelsGen.Add(1)
	model := s.Config.LLM.Providers[provider].Model
	s.publish(teamID, "model_changed", fmt.Sprintf("Euer Agent nutzt ab dem nächsten Lauf das Modell %s.", model),
		map[string]string{"provider": provider, "model": model})
	return nil
}

// SetAllowedModels sets the providers teams may choose besides the default.
// Teams that chose a provider no longer released go back to the default.
func (s *Service) SetAllowedModels(ctx context.Context, providers []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := map[string]bool{}
	for _, p := range providers {
		if !s.configured(p) {
			return userErr("Das Modell „%s“ ist nicht eingerichtet.", p)
		}
		if p != s.defaultProvider() {
			next[p] = true
		}
	}
	names := make([]string, 0, len(next))
	for p := range next {
		names = append(names, p)
	}
	slices.Sort(names)
	raw, _ := json.Marshal(names)
	if err := s.Store.SetSetting(ctx, teamModelsKey, string(raw)); err != nil {
		return err
	}
	revoked := []string{}
	for p := range s.allowed {
		if !next[p] {
			revoked = append(revoked, p)
		}
	}
	s.allowed = next
	s.modelsGen.Add(1)

	def := s.Config.LLM.Providers[s.defaultProvider()].Model
	for _, p := range revoked {
		ids, err := s.Store.ClearTeamProvider(ctx, p)
		if err != nil {
			return err
		}
		model := s.Config.LLM.Providers[p].Model
		if model == "" {
			model = p
		}
		for _, id := range ids {
			s.publish(id, "model_changed", fmt.Sprintf("Der Leitstand hat das Modell %s gesperrt. Euer Agent nutzt ab dem nächsten Lauf wieder %s.", model, def),
				map[string]string{"provider": s.defaultProvider(), "model": def})
		}
	}
	s.Log.Info("team models released", "providers", names)
	return nil
}

// publish shows a notice in a team's log without telling its agent.
func (s *Service) publish(teamID, kind, text string, data any) {
	raw, _ := json.Marshal(data)
	s.Hub.Publish(teamID, live.LogEntry{Source: live.SourceSystem, Kind: kind, Text: text, Data: raw})
}
