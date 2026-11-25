package tools

import (
	"strings"
	"testing"
)

func TestEveryByHandToolHasABlock(t *testing.T) {
	r, _, _ := setup(t, nil)
	if problems := r.checkBlockTexts(); len(problems) > 0 {
		t.Fatalf("block texts: %v", problems)
	}
	for _, tb := range r.ToolBlocks() {
		if tb.Text == "" || tb.Group == "" {
			t.Errorf("%s: no German text or group", tb.Tool)
		}
		if n := strings.Count(tb.Text, "%"); n != len(tb.Fields) {
			t.Errorf("%s: %d placeholders for %d fields", tb.Tool, n, len(tb.Fields))
		}
		// A new block with its defaults is a valid call.
		args := map[string]any{}
		for _, f := range tb.Fields {
			if f.Default != nil && f.Default != "" {
				args[f.Arg] = f.Default
			}
		}
		if err := r.CheckArgs(tb.Tool, args); err != nil {
			t.Errorf("%s with its defaults: %v", tb.Tool, err)
		}
	}
}
