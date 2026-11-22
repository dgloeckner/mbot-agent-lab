package memory

import (
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func TestTrimCutsAtUserMessages(t *testing.T) {
	var h []llm.Message
	for round := 0; round < 5; round++ {
		h = append(h,
			llm.Message{Role: llm.RoleUser, Content: "u"},
			llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "x"}}},
			llm.Message{Role: llm.RoleTool, ToolCallID: "x"},
			llm.Message{Role: llm.RoleAssistant, Content: "a"},
		)
	}
	got := Trim(h, 6)
	if got[0].Role != llm.RoleUser || !strings.HasPrefix(got[0].Content, "(Ältere") {
		t.Fatalf("first kept = %+v", got[0])
	}
	if len(got) != 4 {
		t.Errorf("kept %d messages, want the last round of 4", len(got))
	}
	if len(Trim(h, 100)) != len(h) {
		t.Error("short history must stay untouched")
	}
	if h[16].Content != "u" {
		t.Error("Trim must not modify the original history")
	}
}

func TestFormatNotes(t *testing.T) {
	if FormatNotes(nil) != "Noch keine Notizen." {
		t.Error("empty notes")
	}
	got := FormatNotes([]store.Note{{ID: 3, Text: "Wand links", Author: "agent"}, {ID: 4, Text: "Blau ist hinten", Author: "team"}})
	if got != "- [3] Wand links (von dir)\n- [4] Blau ist hinten (vom Team)" {
		t.Errorf("notes = %q", got)
	}
	var many []store.Note
	for i := 0; i < 100; i++ {
		many = append(many, store.Note{ID: int64(i), Text: strings.Repeat("x", 50)})
	}
	if out := FormatNotes(many); len([]rune(out)) > MaxNotesChars+60 || !strings.Contains(out, "weitere Notizen ausgelassen") {
		t.Errorf("notes not limited: %d chars", len([]rune(out)))
	}
}
