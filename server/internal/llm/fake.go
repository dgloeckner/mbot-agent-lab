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
// an API key or network. It ignores the team prompt.
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
