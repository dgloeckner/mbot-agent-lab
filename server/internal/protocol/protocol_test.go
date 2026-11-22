package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const examples = "../../../docs/examples/protocol"

func readExample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examples, name))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

// TestExamplesRoundTrip decodes each shared example strictly and re-encodes it,
// so field names in Go and in docs/examples/protocol cannot drift apart.
func TestExamplesRoundTrip(t *testing.T) {
	cases := map[string]any{
		"cmd.json":    &Cmd{},
		"result.json": &Result{},
		"event.json":  &Event{},
		"state.json":  &State{},
		"hello.json":  &Hello{},
		"hb.json":     &Heartbeat{},
	}
	for file, v := range cases {
		t.Run(file, func(t *testing.T) {
			raw := readExample(t, file)
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(v); err != nil {
				t.Fatal(err)
			}
			out, err := Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(t, raw, out) {
				t.Errorf("round trip differs:\n in: %s\nout: %s", raw, out)
			}
		})
	}
}

func TestStatusExample(t *testing.T) {
	if got := string(readExample(t, "status.txt")); got != StatusOnline {
		t.Errorf("status example = %q", got)
	}
}

func TestTopics(t *testing.T) {
	if got := Topic("mbot-1", TopicCmd); got != "lab/mbot-1/cmd" {
		t.Errorf("Topic = %q", got)
	}
	id, suffix, ok := ParseTopic("lab/mbot-3/state")
	if !ok || id != "mbot-3" || suffix != "state" {
		t.Errorf("ParseTopic = %q %q %v", id, suffix, ok)
	}
	for _, bad := range []string{"lab/mbot-1", "other/mbot-1/cmd", "lab//cmd", "lab/a/b/c"} {
		if _, _, ok := ParseTopic(bad); ok {
			t.Errorf("ParseTopic(%q) accepted", bad)
		}
	}
}

func TestMarshalSizeLimit(t *testing.T) {
	big := make([]byte, MaxMessageSize)
	for i := range big {
		big[i] = 'x'
	}
	if _, err := Marshal(Cmd{ID: "c", Name: "show_text", Args: map[string]any{"text": string(big)}}); err == nil {
		t.Error("oversized message accepted")
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}
