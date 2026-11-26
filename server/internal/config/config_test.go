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
	if c.Budgets.MaxConversationMessagesPerMinute != DefaultMaxConversationMessages || c.Budgets.MaxTokensPerConversation != DefaultMaxTokensPerConversation {
		t.Errorf("conversation budgets = %+v", c.Budgets)
	}
	if c.Budgets.MaxStepsPerWakeup != DefaultMaxStepsPerWakeup {
		t.Errorf("defaults not applied")
	}
}

func TestParseLeavesOutUnkeyedProviders(t *testing.T) {
	yaml := strings.Replace(validYAML, "    ollama:", "    claude: { kind: anthropic, model: claude-opus-5-5, api_key_env: ANTHROPIC }\n    ollama:", 1)
	c, err := Parse([]byte(yaml), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.LLM.Providers["claude"]; ok {
		t.Errorf("claude without key should be left out: %+v", c.LLM.Providers)
	}
	if len(c.LLM.Unkeyed) != 1 || c.LLM.Unkeyed[0] != "claude" {
		t.Errorf("Unkeyed = %v, want [claude]", c.LLM.Unkeyed)
	}
	if _, ok := c.LLM.Providers["ollama"]; !ok {
		t.Errorf("ollama needs no key and must stay")
	}

	c, err = Parse([]byte(yaml), env(map[string]string{"ANTHROPIC": "k2"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.LLM.Providers["claude"].APIKey != "k2" || len(c.LLM.Unkeyed) != 0 {
		t.Errorf("claude with key: %+v, Unkeyed %v", c.LLM.Providers["claude"], c.LLM.Unkeyed)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, yaml string
		env        map[string]string
		want       string
	}{
		{"missing secret", validYAML, map[string]string{"P2": ""}, "P2"},
		{"missing default key", validYAML, map[string]string{"MISTRAL": ""}, "MISTRAL"},
		{"missing fallback key", strings.Replace(validYAML, "  default: mistral", "  default: ollama\n  fallback: mistral", 1), map[string]string{"MISTRAL": ""}, "MISTRAL"},
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

const robotSetupYAML = `
robot_setup:
  wifi_ssid: lab-guest
  wifi_password_env: WIFI
  broker_host: 192.168.1.10
`

func TestRobotScriptConfig(t *testing.T) {
	c, err := Parse([]byte(validYAML), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RobotScriptConfigFor("mbot-1"); err != ErrNoRobotSetup {
		t.Errorf("without robot_setup: %v", err)
	}
	c, err = Parse([]byte(validYAML+robotSetupYAML), env(map[string]string{"WIFI": "w"}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.RobotScriptConfigFor("mbot-2")
	want := RobotScriptConfig{RobotID: "mbot-2", WiFiSSID: "lab-guest", WiFiPassword: "w",
		BrokerHost: "192.168.1.10", BrokerPort: 1883, BrokerPassword: "p2"}
	if err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
	if _, err := c.RobotScriptConfigFor("mbot-9"); err != ErrUnknownRobot {
		t.Errorf("unknown robot: %v", err)
	}
}

func TestRobotSetupErrors(t *testing.T) {
	tlsYAML := strings.Replace(validYAML, `  mqtt_plain_addr: ":1883"`,
		"  mqtt_tls_addr: \":8883\"\n  tls: { mode: autocert, hosts: [lab.example.org] }", 1)
	cases := []struct {
		name, yaml string
		env        map[string]string
		want       string
	}{
		{"wifi password missing", validYAML + robotSetupYAML, nil, "WIFI"},
		{"no ssid", validYAML + strings.Replace(robotSetupYAML, "wifi_ssid: lab-guest", `wifi_ssid: ""`, 1), map[string]string{"WIFI": "w"}, "wifi_ssid and broker_host"},
		{"tls without tls listener", validYAML + robotSetupYAML + "  broker_tls: true\n", map[string]string{"WIFI": "w"}, "needs server.mqtt_tls_addr"},
		{"tls host not in cert", tlsYAML + robotSetupYAML + "  broker_tls: true\n", map[string]string{"WIFI": "w"}, "not in server.tls.hosts"},
		{"plain without plain listener", tlsYAML + robotSetupYAML, map[string]string{"WIFI": "w"}, "needs server.mqtt_plain_addr"},
		{"bad port", validYAML + robotSetupYAML + "  broker_port: 70000\n", map[string]string{"WIFI": "w"}, "invalid broker_port"},
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
