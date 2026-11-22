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
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of the lab configuration.
type Config struct {
	Server  Server  `yaml:"server"`
	LLM     LLM     `yaml:"llm"`
	Budgets Budgets `yaml:"budgets"`
	Robots  []Robot `yaml:"robots"`
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
}

// Provider is one OpenAI-compatible chat completions endpoint.
type Provider struct {
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`

	APIKey string `yaml:"-"`
}

// Budgets limit what a single agent may consume.
type Budgets struct {
	MaxStepsPerWakeup         int `yaml:"max_steps_per_wakeup"`
	MaxTokensPerRun           int `yaml:"max_tokens_per_run"`
	MaxAgentMessagesPerMinute int `yaml:"max_agent_messages_per_minute"`
}

// Robot is one physical (or simulated) mBot2.
type Robot struct {
	ID          string `yaml:"id"`
	PasswordEnv string `yaml:"password_env"`
	Color       string `yaml:"color"`

	Password string `yaml:"-"`
}

// Defaults applied before validation.
const (
	DefaultHTTPAddr          = ":8080"
	DefaultDBPath            = "lab.db"
	DefaultMaxParallelCalls  = 5
	DefaultMaxStepsPerWakeup = 12
	DefaultMaxTokensPerRun   = 200000
	DefaultMaxAgentMessages  = 6
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
	if c.Server.LabCodeEnv != "" {
		c.Server.LabCode = strings.ToUpper(strings.TrimSpace(get(c.Server.LabCodeEnv, false)))
	}
	for name, p := range c.LLM.Providers {
		// API keys are optional: local providers such as Ollama need none.
		p.APIKey = get(p.APIKeyEnv, false)
		if p.APIKeyEnv != "" && p.APIKey == "" {
			missing = append(missing, p.APIKeyEnv)
		}
		c.LLM.Providers[name] = p
	}
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
	return errors.Join(errs...)
}
