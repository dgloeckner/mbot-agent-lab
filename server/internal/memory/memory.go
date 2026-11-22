// Package memory shapes what the model remembers: the working context of a run
// (a sliding window over the conversation) and the team's notes.
//
// Older parts of a run are left out rather than summarized for now; a summary
// by the model can replace the notice in Trim later.
package memory

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// omitted is prepended when older messages are left out of the context.
const omitted = "(Ältere Nachrichten dieses Laufs sind ausgelassen.)\n\n"

// Trim keeps at most max messages of history, cutting only at a user message
// so tool calls never lose their results. The first kept message notes the cut.
func Trim(history []llm.Message, max int) []llm.Message {
	if max <= 0 || len(history) <= max {
		return history
	}
	for i := len(history) - max; i < len(history); i++ {
		if history[i].Role == llm.RoleUser {
			first := history[i]
			first.Content = omitted + first.Content
			return append([]llm.Message{first}, history[i+1:]...)
		}
	}
	// No user message in the window: keep the newest round as is.
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			return history[i:]
		}
	}
	return history
}

// MaxNotesChars limits how much of the notes goes into every model call.
const MaxNotesChars = 2000

// FormatNotes renders the notes for the system message, newest last.
func FormatNotes(notes []store.Note) string {
	if len(notes) == 0 {
		return "Noch keine Notizen."
	}
	var b strings.Builder
	used := 0
	for i, n := range notes {
		who := "vom Team"
		if n.Author == "agent" {
			who = "von dir"
		}
		line := fmt.Sprintf("- [%d] %s (%s)\n", n.ID, n.Text, who)
		if used+utf8.RuneCountInString(line) > MaxNotesChars {
			fmt.Fprintf(&b, "- (%d weitere Notizen ausgelassen)\n", len(notes)-i)
			break
		}
		used += utf8.RuneCountInString(line)
		b.WriteString(line)
	}
	return strings.TrimRight(b.String(), "\n")
}
