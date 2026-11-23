package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
server:
  http_addr: ":8080"
  mqtt_plain_addr: ":1883"
  admin_password_env: ADMIN
  lab_code_env: LAB_CODE
llm:
  default: mistral
  providers:
    mistral: { base_url: "https://api.mistral.ai/v1", api_key_env: MISTRAL, model: mistral-small-latest }
    ollama:  { base_url: "http://localhost:11434/v1", model: qwen }
robots:
  - { id: mbot-1, password_env: P1, color: red }
  - { id: mbot-2, password_env: P2, color: blue }
`

func env(extra map[string]string) func(string) (string, bool) {
	vals := map[string]string{
		"ADMIN": "a", "MISTRAL": "k", "P1": "p1", "P2": "p2", "LAB_CODE": " dev1 ",
	}
	for k, v := range extra {
		vals[k] = v
	}
	return func(k string) (string, bool) { v, ok := vals[k]; return v, ok }
}

func TestParseValid(t *testing.T) {
	c, err := Parse([]byte(validYAML), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Robots[0].Password != "p1" || c.LLM.Providers["mistral"].APIKey != "k" {
		t.Errorf("secrets not resolved: %+v", c)
	}
	if c.Server.LabCode != "DEV1" {
		t.Errorf("lab code = %q, want DEV1", c.Server.LabCode)
	}
	if c.Budgets.MaxStepsPerWakeup != DefaultMaxStepsPerWakeup {
		t.Errorf("defaults not applied")
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, yaml string
		env        map[string]string
		want       string
	}{
		{"missing secret", validYAML, map[string]string{"P2": ""}, "P2"},
		{"duplicate robot", strings.Replace(validYAML, "id: mbot-2", "id: mbot-1", 1), nil, `duplicate id "mbot-1"`},
		{"unknown field", validYAML + "\nfoo: 1\n", nil, "field foo not found"},
		{"old teams section", validYAML + "\nteams: []\n", nil, "field teams not found"},
		{"claude without key", strings.Replace(validYAML, "    ollama:", "    claude: { kind: anthropic, model: claude-opus-5-5 }\n    ollama:", 1), nil, `provider "claude" needs model and api_key_env`},
		{"claude effort", strings.Replace(validYAML, "    ollama:", "    claude: { kind: anthropic, model: claude-opus-5-5, api_key_env: MISTRAL, effort: fast }\n    ollama:", 1), nil, `unknown effort "fast"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml), env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// TestExampleConfigs makes sure the shipped example configurations stay valid.
func TestExampleConfigs(t *testing.T) {
	files, _ := filepath.Glob("../../../configs/*.example.yaml")
	local, _ := filepath.Glob("../../../deploy/local/*.yaml")
	files = append(files, local...)
	if len(files) == 0 {
		t.Fatal("no example configs found")
	}
	anything := func(string) (string, bool) { return "x", true }
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(raw, anything); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
