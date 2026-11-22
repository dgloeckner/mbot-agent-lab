// Package config loads the lab server configuration from YAML.
//
// Secrets (passwords, tokens, API keys) are never stored in the YAML file itself.
// The file only names the environment variables that hold them; Load resolves them.
// The whole configuration is validated at startup so the server refuses to run with
// an inconsistent setup (for example a robot assigned to two teams).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of the lab configuration.
type Config struct {
	Server   Server             `yaml:"server"`
	LLM      LLM                `yaml:"llm"`
	Budgets  Budgets            `yaml:"budgets"`
	Robots   []Robot            `yaml:"robots"`
	Teams    []Team             `yaml:"teams"`
	Profile  string             `yaml:"profile"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Server holds listener addresses, TLS and storage settings.
type Server struct {
	HTTPAddr         string `yaml:"http_addr"`
	MQTTTLSAddr      string `yaml:"mqtt_tls_addr"`
	MQTTPlainAddr    string `yaml:"mqtt_plain_addr"`
	TLS              TLS    `yaml:"tls"`
	AdminPasswordEnv string `yaml:"admin_password_env"`
	DBPath           string `yaml:"db_path"`

	AdminPassword string `yaml:"-"`
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

// Team is one group of students with its own agent.
type Team struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	TokenEnv string `yaml:"token_env"`
	Color    string `yaml:"color"` // accent of the team page; defaults to the first robot's color

	Token string `yaml:"-"`
}

// TeamColor returns the team's configured color, else the color of the
// alphabetically first robot assigned to it in the active profile.
func (c *Config) TeamColor(teamID string) string {
	if t, ok := c.Team(teamID); ok && t.Color != "" {
		return t.Color
	}
	aliases := c.Profiles[c.Profile][teamID]
	for _, alias := range sortedKeys(aliases) {
		for _, r := range c.Robots {
			if r.ID == aliases[alias] && r.Color != "" {
				return r.Color
			}
		}
	}
	return "blue"
}

// Profile is a complete assignment of robots: team ID -> alias -> robot ID.
type Profile map[string]map[string]string

// Defaults applied before validation.
const (
	DefaultHTTPAddr          = ":8080"
	DefaultDBPath            = "lab.db"
	DefaultMaxParallelCalls  = 5
	DefaultMaxStepsPerWakeup = 12
	DefaultMaxTokensPerRun   = 200000
	DefaultMaxAgentMessages  = 6
)

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	aliasPattern = regexp.MustCompile(`^[a-zäöüß][a-zäöüß0-9_-]{0,15}$`)
)

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
	for i := range c.Teams {
		c.Teams[i].Token = get(c.Teams[i].TokenEnv, true)
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
	teams := map[string]bool{}
	for _, t := range c.Teams {
		if !idPattern.MatchString(t.ID) {
			add("teams: invalid id %q", t.ID)
		}
		if teams[t.ID] {
			add("teams: duplicate id %q", t.ID)
		}
		teams[t.ID] = true
	}

	if _, ok := c.Profiles[c.Profile]; !ok {
		add("profile: active profile %q is not defined", c.Profile)
	}
	for _, name := range sortedKeys(c.Profiles) {
		errs = append(errs, c.Profiles[name].validate(name, robots, teams)...)
	}

	return errors.Join(errs...)
}

// validate enforces the exclusivity rule: every robot has at most one owner.
func (p Profile) validate(name string, robots, teams map[string]bool) []error {
	var errs []error
	owner := map[string]string{}
	for _, team := range sortedKeys(p) {
		if !teams[team] {
			errs = append(errs, fmt.Errorf("profile %s: unknown team %q", name, team))
		}
		for _, alias := range sortedKeys(p[team]) {
			robot := p[team][alias]
			if !aliasPattern.MatchString(alias) {
				errs = append(errs, fmt.Errorf("profile %s: invalid alias %q", name, alias))
			}
			if alias == "all" {
				errs = append(errs, fmt.Errorf("profile %s: alias %q is reserved", name, alias))
			}
			if !robots[robot] {
				errs = append(errs, fmt.Errorf("profile %s: unknown robot %q", name, robot))
			}
			if prev, taken := owner[robot]; taken {
				errs = append(errs, fmt.Errorf("profile %s: robot %q assigned twice (%s and %s.%s)", name, robot, prev, team, alias))
			}
			owner[robot] = team + "." + alias
		}
	}
	return errs
}

// Assignment is one robot's owner within the active profile.
type Assignment struct {
	RobotID string
	TeamID  string
	Alias   string
}

// Assignments returns the active profile as a robot ID -> assignment map.
// Robots without an owner are absent.
func (c *Config) Assignments() map[string]Assignment {
	out := map[string]Assignment{}
	for team, aliases := range c.Profiles[c.Profile] {
		for alias, robot := range aliases {
			out[robot] = Assignment{RobotID: robot, TeamID: team, Alias: alias}
		}
	}
	return out
}

// Team returns the team with the given ID.
func (c *Config) Team(id string) (Team, bool) {
	for _, t := range c.Teams {
		if t.ID == id {
			return t, true
		}
	}
	return Team{}, false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
