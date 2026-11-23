package lab

import (
	"context"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
)

// Conversations: while no run is active, a team can talk with its agent. The
// control room can switch that off for the whole lab, for example during a
// competition. It is on unless switched off.

// conversationKey stores "off" while conversations are switched off.
const conversationKey = "conversation"

func (s *Service) loadConversation(ctx context.Context) error {
	v, err := s.Store.Setting(ctx, conversationKey)
	if err != nil {
		return err
	}
	s.conversationOff.Store(v == "off")
	return nil
}

// ConversationEnabled reports whether teams may talk with a stopped agent.
func (s *Service) ConversationEnabled() bool { return !s.conversationOff.Load() }

// SetConversationEnabled switches conversations on or off for every team and
// tells the teams.
func (s *Service) SetConversationEnabled(ctx context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ConversationEnabled() == on {
		return nil
	}
	v := "on"
	if !on {
		v = "off"
	}
	if err := s.Store.SetSetting(ctx, conversationKey, v); err != nil {
		return err
	}
	s.conversationOff.Store(!on)
	teams, err := s.Store.Teams(ctx)
	if err != nil {
		return err
	}
	text := "Der Leitstand hat das Gespräch mit eurem Agenten abgeschaltet. Im Lauf könnt ihr ihm weiter schreiben."
	if on {
		text = "Der Leitstand hat das Gespräch mit eurem Agenten eingeschaltet. Ohne Lauf könnt ihr ihm hier Fragen stellen."
	}
	for _, t := range teams {
		s.Hub.Publish(t.ID, live.LogEntry{Source: live.SourceSystem, Kind: "info", Text: text, Channel: live.ChannelConversation})
	}
	s.Log.Info("conversations switched", "on", on)
	return nil
}
