// Package config loads the lab server configuration from YAML.
//
// Secrets (passwords, tokens, API keys) are never stored in the YAML file itself.
// The file only names the environment variables that hold them; Load resolves them.
// The whole configuration is validated at startup so the server refuses to run with
// an inconsistent setup.
//
// Teams and robot assignments are not configured here: teams join with the lab
// code and the control room assigns robots; both live in the database.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of the lab configuration.
type Config struct {
	Server  Server  `yaml:"server"`
	LLM     LLM     `yaml:"llm"`
	Budgets Budgets `yaml:"budgets"`
	Robots  []Robot `yaml:"robots"`

	// RobotSetup is optional: without it the server hands out no robot setups (robot:flash).
	RobotSetup *RobotSetup `yaml:"robot_setup"`
}

// Server holds listener addresses, TLS and storage settings.
type Server struct {
	HTTPAddr         string `yaml:"http_addr"`
	MQTTTLSAddr      string `yaml:"mqtt_tls_addr"`
	MQTTPlainAddr    string `yaml:"mqtt_plain_addr"`
	TLS              TLS    `yaml:"tls"`
	AdminPasswordEnv string `yaml:"admin_password_env"`
	LabCodeEnv       string `yaml:"lab_code_env"` // optional: fixes the lab code, e.g. for development
	DBPath           string `yaml:"db_path"`

	AdminPassword string `yaml:"-"`
	LabCode       string `yaml:"-"`
}

// TLS modes.
const (
	TLSOff      = "off"
	TLSAutocert = "autocert"
)

// TLS configures certificates for HTTPS and MQTT over TLS.
type TLS struct {
	Mode     string   `yaml:"mode"`      // off | autocert
	Hosts    []string `yaml:"hosts"`     // host names for autocert
	CacheDir string   `yaml:"cache_dir"` // autocert certificate cache
}

// LLM configures the language model providers.
type LLM struct {
	Default          string              `yaml:"default"`
	Fallback         string              `yaml:"fallback"`
	Providers        map[string]Provider `yaml:"providers"`
	MaxParallelCalls int                 `yaml:"max_parallel_calls"`

	// Unkeyed lists providers left out because their API key variable is not
	// set. Only the default and fallback providers must have their key.
	Unkeyed []string `yaml:"-"`
}

// Provider is one model endpoint.
type Provider struct {
	Kind      string  `yaml:"kind"`     // openai (default), anthropic (Claude) or fake (scripted test model)
	BaseURL   string  `yaml:"base_url"` // anthropic: empty for the Claude API
	APIKeyEnv string  `yaml:"api_key_env"`
	Model     string  `yaml:"model"`
	PriceIn   float64 `yaml:"price_in"`  // EUR per million input tokens, for the cost display
	PriceOut  float64 `yaml:"price_out"` // EUR per million output tokens
	Effort    string  `yaml:"effort"`    // anthropic: low, medium, high, xhigh or max; empty = model default

	APIKey string `yaml:"-"`
}

// Provider kinds.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderFake      = "fake"
)

// FakeModel is the model name of a fake provider that names none.
const FakeModel = "testmodell"

// Budgets limit what a single agent may consume.
type Budgets struct {
	MaxStepsPerWakeup         int `yaml:"max_steps_per_wakeup"`
	MaxTokensPerRun           int `yaml:"max_tokens_per_run"`
	MaxAgentMessagesPerMinute int `yaml:"max_agent_messages_per_minute"`
	MaxRunMinutes             int `yaml:"max_run_minutes"`
	MaxHistoryMessages        int `yaml:"max_history_messages"` // older ones are left out of the context

	// Conversations: a team talks with its agent while no run is active.
	MaxConversationMessagesPerMinute int `yaml:"max_conversation_messages_per_minute"`
	MaxTokensPerConversation         int `yaml:"max_tokens_per_conversation"` // then a new conversation begins
}

// Robot is one physical (or simulated) mBot2.
type Robot struct {
	ID          string `yaml:"id"`
	PasswordEnv string `yaml:"password_env"`
	Color       string `yaml:"color"`

	Password string `yaml:"-"`
}

// RobotSetup tells how robots reach the Wi-Fi and the broker. `labctl robot flash` gets it from
// the server (together with the robot's password) and inlines it into the robot's script.
type RobotSetup struct {
	WiFiSSID        string `yaml:"wifi_ssid"`
	WiFiPasswordEnv string `yaml:"wifi_password_env"` // empty for an open network
	BrokerHost      string `yaml:"broker_host"`
	BrokerPort      int    `yaml:"broker_port"` // default 8883 with TLS, else 1883
	BrokerTLS       bool   `yaml:"broker_tls"`

	WiFiPassword string `yaml:"-"`
}

// RobotScriptConfig is everything one robot's script needs (the names in
// robot/config_example.py); the server hands it to labctl, which inlines it into the bundle.
type RobotScriptConfig struct {
	RobotID        string `json:"robot_id"`
	WiFiSSID       string `json:"wifi_ssid"`
	WiFiPassword   string `json:"wifi_password"`
	BrokerHost     string `json:"broker_host"`
	BrokerPort     int    `json:"broker_port"`
	BrokerTLS      bool   `json:"broker_tls"`
	BrokerPassword string `json:"broker_password"`
}

// Errors of RobotScriptConfigFor.
var (
	ErrNoRobotSetup = errors.New("config: no robot_setup section")
	ErrUnknownRobot = errors.New("config: unknown robot")
)

// RobotScriptConfigFor returns the script configuration of robot id.
func (c *Config) RobotScriptConfigFor(id string) (RobotScriptConfig, error) {
	if c.RobotSetup == nil {
		return RobotScriptConfig{}, ErrNoRobotSetup
	}
	for _, r := range c.Robots {
		if r.ID == id {
			s := c.RobotSetup
			return RobotScriptConfig{
				RobotID: r.ID, WiFiSSID: s.WiFiSSID, WiFiPassword: s.WiFiPassword,
				BrokerHost: s.BrokerHost, BrokerPort: s.BrokerPort, BrokerTLS: s.BrokerTLS,
				BrokerPassword: r.Password,
			}, nil
		}
	}
	return RobotScriptConfig{}, ErrUnknownRobot
}

// Defaults applied before validation.
const (
	DefaultHTTPAddr          = ":8080"
	DefaultDBPath            = "lab.db"
	DefaultMaxParallelCalls  = 5
	DefaultMaxStepsPerWakeup = 12
	DefaultMaxTokensPerRun   = 200000
	DefaultMaxAgentMessages  = 6
	DefaultMaxRunMinutes     = 20
	DefaultMaxHistory        = 40

	DefaultMaxConversationMessages  = 6
	DefaultMaxTokensPerConversation = 100000
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Load reads, resolves and validates the configuration at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw, os.LookupEnv)
}

// Parse decodes YAML, resolves secrets through lookup and validates the result.
// lookup has the signature of os.LookupEnv so tests can inject values.
func Parse(raw []byte, lookup func(string) (string, bool)) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c.applyDefaults()
	if err := c.resolveSecrets(lookup); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	for _, name := range c.LLM.Unkeyed {
		delete(c.LLM.Providers, name)
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Server.HTTPAddr == "" {
		c.Server.HTTPAddr = DefaultHTTPAddr
	}
	if c.Server.DBPath == "" {
		c.Server.DBPath = DefaultDBPath
	}
	if c.Server.TLS.Mode == "" {
		c.Server.TLS.Mode = TLSOff
	}
	if c.LLM.MaxParallelCalls == 0 {
		c.LLM.MaxParallelCalls = DefaultMaxParallelCalls
	}
	if c.Budgets.MaxStepsPerWakeup == 0 {
		c.Budgets.MaxStepsPerWakeup = DefaultMaxStepsPerWakeup
	}
	if c.Budgets.MaxTokensPerRun == 0 {
		c.Budgets.MaxTokensPerRun = DefaultMaxTokensPerRun
	}
	if c.Budgets.MaxAgentMessagesPerMinute == 0 {
		c.Budgets.MaxAgentMessagesPerMinute = DefaultMaxAgentMessages
	}
	if c.Budgets.MaxRunMinutes == 0 {
		c.Budgets.MaxRunMinutes = DefaultMaxRunMinutes
	}
	if c.Budgets.MaxHistoryMessages == 0 {
		c.Budgets.MaxHistoryMessages = DefaultMaxHistory
	}
	if c.Budgets.MaxConversationMessagesPerMinute == 0 {
		c.Budgets.MaxConversationMessagesPerMinute = DefaultMaxConversationMessages
	}
	if c.Budgets.MaxTokensPerConversation == 0 {
		c.Budgets.MaxTokensPerConversation = DefaultMaxTokensPerConversation
	}
	if s := c.RobotSetup; s != nil && s.BrokerPort == 0 {
		s.BrokerPort = 1883
		if s.BrokerTLS {
			s.BrokerPort = 8883
		}
	}
	for name, p := range c.LLM.Providers {
		if p.Kind == "" {
			p.Kind = ProviderOpenAI
		}
		if p.Kind == ProviderFake && p.Model == "" {
			p.Model = FakeModel
		}
		c.LLM.Providers[name] = p
	}
}

// resolveSecrets reads every *_env reference. Missing required secrets are
// collected so the operator sees all of them at once.
func (c *Config) resolveSecrets(lookup func(string) (string, bool)) error {
	var missing []string
	get := func(env string, required bool) string {
		if env == "" {
			if required {
				missing = append(missing, "(no variable name given)")
			}
			return ""
		}
		v, ok := lookup(env)
		if (!ok || v == "") && required {
			missing = append(missing, env)
		}
		return v
	}

	c.Server.AdminPassword = get(c.Server.AdminPasswordEnv, true)
	for i := range c.Robots {
		c.Robots[i].Password = get(c.Robots[i].PasswordEnv, true)
	}
	if c.RobotSetup != nil && c.RobotSetup.WiFiPasswordEnv != "" {
		c.RobotSetup.WiFiPassword = get(c.RobotSetup.WiFiPasswordEnv, true)
	}
	if c.Server.LabCodeEnv != "" {
		c.Server.LabCode = strings.ToUpper(strings.TrimSpace(get(c.Server.LabCodeEnv, false)))
	}
	for name, p := range c.LLM.Providers {
		// Local providers such as Ollama name no key variable. A named key is
		// required only for the providers runs actually use; any other
		// provider without its key is left out (see Unkeyed).
		p.APIKey = get(p.APIKeyEnv, false)
		if p.APIKeyEnv != "" && p.APIKey == "" {
			if name == c.LLM.Default || name == c.LLM.Fallback {
				missing = append(missing, p.APIKeyEnv)
			} else {
				c.LLM.Unkeyed = append(c.LLM.Unkeyed, name)
			}
		}
		c.LLM.Providers[name] = p
	}
	slices.Sort(c.LLM.Unkeyed)
	if len(missing) > 0 {
		return fmt.Errorf("config: missing environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Validate checks the configuration for consistency.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if c.Server.MQTTPlainAddr == "" && c.Server.MQTTTLSAddr == "" {
		add("server: at least one of mqtt_plain_addr or mqtt_tls_addr is required")
	}
	switch c.Server.TLS.Mode {
	case TLSOff:
		if c.Server.MQTTTLSAddr != "" {
			add("server: mqtt_tls_addr requires tls.mode autocert")
		}
	case TLSAutocert:
		if len(c.Server.TLS.Hosts) == 0 {
			add("server: tls.mode autocert requires tls.hosts")
		}
	default:
		add("server: unknown tls.mode %q", c.Server.TLS.Mode)
	}

	for name, p := range c.LLM.Providers {
		switch p.Kind {
		case ProviderOpenAI:
			if p.BaseURL == "" || p.Model == "" {
				add("llm: provider %q needs base_url and model", name)
			}
		case ProviderAnthropic:
			if p.Model == "" || p.APIKeyEnv == "" {
				add("llm: provider %q needs model and api_key_env", name)
			}
			switch p.Effort {
			case "", "low", "medium", "high", "xhigh", "max":
			default:
				add("llm: provider %q has unknown effort %q", name, p.Effort)
			}
		case ProviderFake:
		default:
			add("llm: provider %q has unknown kind %q", name, p.Kind)
		}
	}
	if len(c.LLM.Providers) > 0 {
		if _, ok := c.LLM.Providers[c.LLM.Default]; !ok {
			add("llm: default provider %q is not defined", c.LLM.Default)
		}
		if c.LLM.Fallback != "" {
			if _, ok := c.LLM.Providers[c.LLM.Fallback]; !ok {
				add("llm: fallback provider %q is not defined", c.LLM.Fallback)
			}
		}
	}

	if len(c.Robots) == 0 {
		add("robots: at least one robot is required")
	}
	robots := map[string]bool{}
	for _, r := range c.Robots {
		if !idPattern.MatchString(r.ID) {
			add("robots: invalid id %q", r.ID)
		}
		if robots[r.ID] {
			add("robots: duplicate id %q", r.ID)
		}
		robots[r.ID] = true
	}
	if s := c.RobotSetup; s != nil {
		if s.WiFiSSID == "" || s.BrokerHost == "" {
			add("robot_setup: wifi_ssid and broker_host are required")
		}
		if s.BrokerPort < 1 || s.BrokerPort > 65535 {
			add("robot_setup: invalid broker_port %d", s.BrokerPort)
		}
		switch {
		case s.BrokerTLS && c.Server.MQTTTLSAddr == "":
			add("robot_setup: broker_tls needs server.mqtt_tls_addr")
		case s.BrokerTLS && !slices.Contains(c.Server.TLS.Hosts, s.BrokerHost):
			add("robot_setup: broker_host %q is not in server.tls.hosts, its certificate would not match", s.BrokerHost)
		case !s.BrokerTLS && c.Server.MQTTPlainAddr == "":
			add("robot_setup: broker_tls false needs server.mqtt_plain_addr")
		}
	}
	return errors.Join(errs...)
}
