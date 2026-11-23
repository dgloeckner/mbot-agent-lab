package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Fake is a scripted stand-in for a language model. It explores with the real
// tools (read sensors, drive to the next wall, turn, repeat) and finishes after
// a few moves, so the agent runtime can be developed and demonstrated without
// an API key or network. It ignores the team prompt. Without the finish tool
// it is in a conversation (no run active): it answers in text, reads the
// sensors or writes a note, and never moves a robot.
type Fake struct {
	// Delay simulates thinking time; zero means a short default.
	Delay time.Duration
}

const fakeMaxMoves = 8

func (f Fake) Chat(ctx context.Context, req Request) (Response, error) {
	d := f.Delay
	if d == 0 {
		d = 400 * time.Millisecond
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}

	has := map[string]Tool{}
	for _, t := range req.Tools {
		has[t.Name] = t
	}
	moves, lastTool, lastResult := 0, "", ""
	assistantSeen := false
	for _, m := range req.Messages {
		switch m.Role {
		case RoleAssistant:
			assistantSeen = true
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == "drive_until" || tc.Function.Name == "turn" || tc.Function.Name == "turn_until_clear" {
					moves++
				}
			}
		case RoleTool:
			lastTool, lastResult = m.Name, m.Content
		}
	}
	last := req.Messages[len(req.Messages)-1]

	reply := func(text, tool string, args map[string]any) (Response, error) {
		msg := Message{Role: RoleAssistant, Content: text}
		if tool != "" {
			if _, ok := has[tool]; !ok {
				msg.Content = text + " (Das Werkzeug " + tool + " fehlt mir gerade.)"
			} else {
				if robot := firstRobot(has[tool]); robot != "" {
					args["robot"] = robot
				}
				raw, _ := json.Marshal(args)
				msg.ToolCalls = []ToolCall{{ID: fmt.Sprintf("f%08d", len(req.Messages)), Type: "function", Function: FunctionCall{Name: tool, Arguments: string(raw)}}}
			}
		}
		in := 0
		for _, m := range req.Messages {
			in += len(m.Content) / 4
		}
		return Response{Message: msg, Usage: Usage{PromptTokens: in + 300, CompletionTokens: len(text)/4 + 20}}, nil
	}

	if _, run := has["finish"]; !run {
		return f.converse(req, lastTool, lastResult, reply)
	}

	// A new instruction is the start of the run or a message from the team.
	// Robot events slipped in between tool rounds just continue the plan.
	if last.Role == RoleUser && (!assistantSeen || strings.Contains(last.Content, `"von":"Team"`)) {
		lower := strings.ToLower(last.Content)
		if strings.Contains(lower, "stopp") || strings.Contains(lower, "halt") {
			return reply("Verstanden, ich halte an.", "stop", map[string]any{})
		}
		if moves >= fakeMaxMoves {
			return reply("Ich habe meine Testrunde schon beendet und warte.", "", nil)
		}
		return reply("Ich bin das Testmodell. Ich schaue mich zuerst um.", "read_sensors", map[string]any{})
	}

	if moves >= fakeMaxMoves {
		return reply("Genug erkundet.", "finish", map[string]any{
			"report": fmt.Sprintf("Testlauf: %d Bewegungen. Ich bin bis zur nächsten Wand gefahren und habe mich gedreht.", moves),
		})
	}
	var res struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
		Color  string `json:"color"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal([]byte(lastResult), &res)
	switch lastTool {
	case "drive_until":
		if res.Reason == "color" && res.Color != "" && res.Color != "white" {
			return reply("Da ist eine Farbe: "+res.Color+".", "finish", map[string]any{"report": "Ich habe das Feld " + res.Color + " gefunden."})
		}
		if _, ok := has["turn_until_clear"]; ok {
			return reply("Wand oder Ende der Strecke. Ich drehe mich, bis der Weg frei ist.", "turn_until_clear", map[string]any{"direction": "right", "min_cm": 40})
		}
		return reply("Wand oder Ende der Strecke. Ich drehe mich.", "turn", map[string]any{"degrees": 90})
	case "turn", "turn_until_clear", "read_sensors":
		return reply("Ich fahre, bis eine Wand kommt.", "drive_until", map[string]any{"max_cm": 120, "stop_below_cm": 15, "stop_on_color": "blue"})
	case "stop":
		return reply("Ich stehe und warte auf eure Nachricht.", "", nil)
	default:
		return reply("Ich warte.", "", nil)
	}
}

// converse answers a team message in a conversation. It only ever calls
// read_sensors or remember.
func (f Fake) converse(req Request, lastTool, lastResult string, reply func(string, string, map[string]any) (Response, error)) (Response, error) {
	last := req.Messages[len(req.Messages)-1]
	if last.Role == RoleTool {
		switch lastTool {
		case "read_sensors":
			var st struct {
				Distance float64 `json:"distance_cm"`
				Battery  float64 `json:"battery"`
				Error    string  `json:"error"`
			}
			_ = json.Unmarshal([]byte(lastResult), &st)
			if st.Error != "" {
				return reply("Ich konnte die Sensoren nicht lesen: "+st.Error, "", nil)
			}
			return reply(fmt.Sprintf("Vorne ist %.0f cm Platz, der Akku hat %.0f %%.", st.Distance, st.Battery), "", nil)
		case "remember":
			return reply("Gemerkt. Die Notiz sehe ich auch im nächsten Lauf.", "", nil)
		}
		return reply("Das ging nicht. Im Gespräch kann ich nur Sensoren lesen und Notizen anlegen.", "", nil)
	}
	question := lastTeamText(last.Content)
	lower := strings.ToLower(question)
	switch {
	case strings.Contains(lower, "merk"):
		return reply("", "remember", map[string]any{"text": clampText(question, 200)})
	case strings.Contains(lower, "abstand") || strings.Contains(lower, "wand") || strings.Contains(lower, "sensor") || strings.Contains(lower, "akku"):
		return reply("", "read_sensors", map[string]any{})
	}
	return reply("Ich bin das Testmodell im Gespräch. Gerade läuft kein Lauf, deshalb bewege ich keinen Roboter. "+
		"Im nächsten Lauf würde ich mich mit read_sensors umsehen und mit drive_until bis zur nächsten Wand fahren.", "", nil)
}

// lastTeamText returns the text of the newest inbox line from the team.
func lastTeamText(inbox string) string {
	lines := strings.Split(inbox, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var l struct {
			From string `json:"von"`
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(lines[i]), &l) == nil && l.From == "Team" {
			return l.Text
		}
	}
	return ""
}

func clampText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// firstRobot returns the first allowed value of a tool's "robot" parameter.
func firstRobot(t Tool) string {
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(t.Parameters, &schema)
	if p, ok := schema.Properties["robot"]; ok && len(p.Enum) > 0 {
		return p.Enum[0]
	}
	return ""
}
