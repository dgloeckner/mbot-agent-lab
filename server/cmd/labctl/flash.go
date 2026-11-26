package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

const defaultServer = "http://127.0.0.1:8080"

func flash(id, server, port string, wait time.Duration) error {
	c, err := newLabClient(server)
	if err != nil {
		return err
	}
	pw, err := adminPassword()
	if err != nil {
		return err
	}
	if err := c.login(pw); err != nil {
		return err
	}
	cfg, err := c.setup(id)
	if err != nil {
		return err
	}
	before, err := c.robot(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Setup for %s from %s: Wi-Fi %q, broker %s:%d (TLS %v)\n",
		id, c.base, cfg.WiFiSSID, cfg.BrokerHost, cfg.BrokerPort, cfg.BrokerTLS)

	src, err := bundleWith(cfg)
	if err != nil {
		return err
	}
	if err := uploadAndRestart("the bundle for "+id, src, port); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Waiting up to %s for %s to connect to %s ...\n", wait, id, c.base)
	r, err := waitOnline(func() (adminRobot, error) { return c.robot(id) }, before.HelloAt, wait, 2*time.Second)
	if err != nil {
		return err
	}
	mem := ""
	if r.MemFree >= 0 {
		mem = fmt.Sprintf(", %d KB free", r.MemFree/1024)
	}
	fmt.Fprintf(os.Stderr, "Done: %s is online (script %s%s).\n", id, r.Script, mem)
	return nil
}

// labClient talks to the admin API of the lab server.
type labClient struct {
	base *url.URL
	http *http.Client
}

func newLabClient(server string) (*labClient, error) {
	if server == "" {
		server = defaultServer
	}
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("--server must look like https://lab.example.org, got %q", server)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("refusing to send the admin password and robot secrets over plain http to %s: use https", u.Host)
	}
	jar, _ := cookiejar.New(nil)
	return &labClient{base: u, http: &http.Client{Jar: jar, Timeout: 15 * time.Second}}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *labClient) do(method, path string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, c.base.String()+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("lab server: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return &httpError{Status: resp.StatusCode, Msg: e.Error}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// httpError is a non-2xx answer; Msg is the server's (German) error text.
type httpError struct {
	Status int
	Msg    string
}

func (e *httpError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("lab server answered %d", e.Status)
	}
	return fmt.Sprintf("lab server answered %d: %s", e.Status, e.Msg)
}

func (c *labClient) login(password string) error {
	body, _ := json.Marshal(map[string]string{"password": password})
	err := c.do("POST", "/admin/api/login", bytes.NewReader(body), nil)
	var he *httpError
	if errors.As(err, &he) && he.Status == http.StatusUnauthorized {
		return errors.New("wrong admin password")
	}
	return err
}

func (c *labClient) setup(id string) (config.RobotScriptConfig, error) {
	var cfg config.RobotScriptConfig
	err := c.do("GET", "/admin/api/robots/"+url.PathEscape(id)+"/setup", nil, &cfg)
	var he *httpError
	if errors.As(err, &he) {
		switch he.Status {
		case http.StatusNotFound:
			return cfg, fmt.Errorf("the lab server has no robot %q (robots: in lab.yaml)", id)
		case http.StatusConflict:
			return cfg, errors.New("the lab server's lab.yaml has no robot_setup section (Wi-Fi and broker for robots)")
		}
	}
	return cfg, err
}

// adminRobot is the part of a robot in /admin/api/state that labctl reads.
type adminRobot struct {
	ID       string    `json:"id"`
	Online   bool      `json:"online"`
	Accepted bool      `json:"accepted"`
	Script   string    `json:"script"`
	MemFree  int       `json:"mem_free"`
	HelloAt  time.Time `json:"hello_at"`
}

func (c *labClient) robot(id string) (adminRobot, error) {
	var st struct {
		Robots []adminRobot `json:"robots"`
	}
	if err := c.do("GET", "/admin/api/state", nil, &st); err != nil {
		return adminRobot{}, err
	}
	for _, r := range st.Robots {
		if r.ID == id {
			return r, nil
		}
	}
	return adminRobot{}, fmt.Errorf("the lab server has no robot %q", id)
}

// waitOnline polls until the robot sends a hello after prev (the server's time of its last hello
// before the upload, so clocks of laptop and server need not agree).
func waitOnline(get func() (adminRobot, error), prev time.Time, wait, every time.Duration) (adminRobot, error) {
	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		r, err := get()
		switch {
		case err != nil:
			lastErr = err
		case r.HelloAt.After(prev) && r.Online && !r.Accepted:
			return r, fmt.Errorf("%s connected but speaks a protocol version the server does not support: bundle robot/src from the same version as the server", r.ID)
		case r.HelloAt.After(prev) && r.Online:
			return r, nil
		}
		if time.Now().After(deadline) {
			msg := fmt.Sprintf("%s did not connect within %s. Check:\n"+
				"  - the robot's display (Wi-Fi or broker error)\n"+
				"  - wifi_ssid and the Wi-Fi password in robot_setup on the server (2.4 GHz only)\n"+
				"  - that broker_host and broker_port in robot_setup are reachable from the robots' Wi-Fi", r.ID, wait)
			if lastErr != nil {
				msg += "\nlast error: " + lastErr.Error()
			}
			return r, errors.New(msg)
		}
		time.Sleep(every)
	}
}

// renderConfig writes cfg as the config module the robot script imports (the names in
// robot/config_example.py). JSON string literals are valid Python string literals.
func renderConfig(cfg config.RobotScriptConfig) []byte {
	str := func(s string) string {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(s)
		return strings.TrimSpace(b.String())
	}
	pyBool := map[bool]string{true: "True", false: "False"}
	var b strings.Builder
	b.WriteString("# Generated by labctl from the lab server. Contains secrets: never commit.\n")
	fmt.Fprintf(&b, "ROBOT_ID = %s\n", str(cfg.RobotID))
	fmt.Fprintf(&b, "WIFI_SSID = %s\n", str(cfg.WiFiSSID))
	fmt.Fprintf(&b, "WIFI_PASSWORD = %s\n", str(cfg.WiFiPassword))
	fmt.Fprintf(&b, "BROKER_HOST = %s\n", str(cfg.BrokerHost))
	fmt.Fprintf(&b, "BROKER_PORT = %s\n", strconv.Itoa(cfg.BrokerPort))
	fmt.Fprintf(&b, "BROKER_TLS = %s\n", pyBool[cfg.BrokerTLS])
	fmt.Fprintf(&b, "BROKER_PASSWORD = %s\n", str(cfg.BrokerPassword))
	return []byte(b.String())
}

// bundleWith runs robot/tools/bundle.py with cfg inlined and returns the bundle. Config and
// bundle hold secrets: they live in a private temporary directory that is removed afterwards.
func bundleWith(cfg config.RobotScriptConfig) ([]byte, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "labctl-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	cfgPath, out := filepath.Join(dir, "config.py"), filepath.Join(dir, "main.py")
	if err := os.WriteFile(cfgPath, renderConfig(cfg), 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("python3", filepath.Join(root, "robot", "tools", "bundle.py"), "--config", cfgPath, "--out", out)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	return os.ReadFile(out)
}

// repoRoot finds the checkout from the working directory upwards.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "robot", "tools", "bundle.py")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run labctl inside the mbot-agent-lab checkout: robot/tools/bundle.py not found")
		}
		dir = parent
	}
}

// adminPassword comes from LAB_ADMIN_PASSWORD or is asked for without echo.
func adminPassword() (string, error) {
	if pw := os.Getenv("LAB_ADMIN_PASSWORD"); pw != "" {
		return pw, nil
	}
	pw, err := readPassword("Admin password of the lab server: ")
	if err != nil {
		return "", fmt.Errorf("cannot ask for the admin password (%v): set LAB_ADMIN_PASSWORD", err)
	}
	return pw, nil
}
