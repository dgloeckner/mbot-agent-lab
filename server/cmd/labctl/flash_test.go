package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

var tricky = config.RobotScriptConfig{
	RobotID: "mbot-7", WiFiSSID: "Gäste WLAN", WiFiPassword: `a"b\c'd` + "\n€",
	BrokerHost: "mqtt.example.org", BrokerPort: 8883, BrokerTLS: true, BrokerPassword: "p#w # x",
}

// TestRenderConfigIsTheRobotsConfig checks that the rendered module defines exactly the names of
// robot/config_example.py and that Python reads the values back unchanged.
func TestRenderConfigIsTheRobotsConfig(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(filepath.Join(root, "robot", "config_example.py"))
	if err != nil {
		t.Fatal(err)
	}
	names := regexp.MustCompile(`(?m)^([A-Z_]+)\s*=`)
	var want []string
	for _, m := range names.FindAllStringSubmatch(string(example), -1) {
		want = append(want, m[1])
	}
	slices.Sort(want)

	path := filepath.Join(t.TempDir(), "config.py")
	if err := os.WriteFile(path, renderConfig(tricky), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("python3", "-I", "-c",
		"import json, runpy, sys; g = runpy.run_path(sys.argv[1]); "+
			"print(json.dumps({k: v for k, v in g.items() if k.isupper()}))", path).Output()
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, want) {
		t.Errorf("names %v, config_example.py has %v", keys, want)
	}
	checks := map[string]any{
		"ROBOT_ID": tricky.RobotID, "WIFI_SSID": tricky.WiFiSSID, "WIFI_PASSWORD": tricky.WiFiPassword,
		"BROKER_HOST": tricky.BrokerHost, "BROKER_PORT": float64(8883), "BROKER_TLS": true,
		"BROKER_PASSWORD": tricky.BrokerPassword,
	}
	for k, v := range checks {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
}

func TestBundleWithInlinesConfig(t *testing.T) {
	src, err := bundleWith(tricky)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `ROBOT_ID = "mbot-7"`) || strings.Contains(string(src), "from config import") {
		t.Errorf("config not inlined")
	}
}

func TestNewLabClientRefusesPlainHTTPToRemote(t *testing.T) {
	for server, ok := range map[string]bool{
		"":                         true,
		"http://127.0.0.1:8080":    true,
		"http://localhost:8080":    true,
		"https://lab.example.org":  true,
		"https://lab.example.org/": true,
		"http://lab.example.org":   false,
		"http://192.168.1.10:8080": false,
		"lab.example.org":          false,
		"ftp://lab.example.org":    false,
	} {
		if _, err := newLabClient(server); (err == nil) != ok {
			t.Errorf("%q: err %v", server, err)
		}
	}
}

// fakeLab is the admin API of the lab server as labctl uses it.
type fakeLab struct {
	states []adminRobot // served one after another by /admin/api/state, the last one repeats
	calls  int
}

func (f *fakeLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	loggedIn := func() bool {
		c, err := r.Cookie("lab_admin")
		return err == nil && c.Value == "ok"
	}
	writeJSON := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.URL.Path == "/admin/api/login":
		var req struct{ Password string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Password != "pw" {
			writeJSON(401, map[string]string{"error": "Das Passwort stimmt nicht."})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "lab_admin", Value: "ok", Path: "/admin"})
		w.WriteHeader(204)
	case !loggedIn():
		writeJSON(401, map[string]string{"error": "Bitte meldet euch im Leitstand an."})
	case r.URL.Path == "/admin/api/robots/mbot-1/setup":
		writeJSON(200, tricky)
	case r.URL.Path == "/admin/api/robots/mbot-2/setup":
		writeJSON(409, map[string]string{"error": "In der Server-Konfiguration fehlt robot_setup."})
	case strings.HasPrefix(r.URL.Path, "/admin/api/robots/"):
		writeJSON(404, map[string]string{"error": "Diesen Roboter gibt es nicht."})
	case r.URL.Path == "/admin/api/state":
		st := f.states[min(f.calls, len(f.states)-1)]
		f.calls++
		writeJSON(200, map[string]any{"robots": []adminRobot{st}})
	default:
		http.NotFound(w, r)
	}
}

func TestLabClient(t *testing.T) {
	old := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	lab := &fakeLab{states: []adminRobot{{ID: "mbot-1", Online: true, Accepted: true, HelloAt: old}}}
	srv := httptest.NewServer(lab)
	defer srv.Close()

	c, err := newLabClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.setup("mbot-1"); err == nil {
		t.Error("setup without login must fail")
	}
	if err := c.login("falsch"); err == nil || err.Error() != "wrong admin password" {
		t.Errorf("wrong password: %v", err)
	}
	if err := c.login("pw"); err != nil {
		t.Fatal(err)
	}
	if cfg, err := c.setup("mbot-1"); err != nil || cfg != tricky {
		t.Errorf("setup: %+v, %v", cfg, err)
	}
	if _, err := c.setup("mbot-2"); err == nil || !strings.Contains(err.Error(), "no robot_setup") {
		t.Errorf("setup without robot_setup: %v", err)
	}
	if _, err := c.setup("mbot-9"); err == nil || !strings.Contains(err.Error(), `no robot "mbot-9"`) {
		t.Errorf("unknown robot: %v", err)
	}
	if r, err := c.robot("mbot-1"); err != nil || !r.HelloAt.Equal(old) {
		t.Errorf("robot: %+v, %v", r, err)
	}
}

func TestWaitOnline(t *testing.T) {
	old := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	seq := func(states ...adminRobot) func() (adminRobot, error) {
		i := 0
		return func() (adminRobot, error) {
			r := states[min(i, len(states)-1)]
			i++
			if r.ID == "" {
				return r, errors.New("network down")
			}
			return r, nil
		}
	}
	stale := adminRobot{ID: "mbot-1", Online: true, Accepted: true, HelloAt: old}
	offline := adminRobot{ID: "mbot-1", HelloAt: old}
	fresh := adminRobot{ID: "mbot-1", Online: true, Accepted: true, HelloAt: old.Add(time.Minute), Script: "0.1.0"}

	// Still shows the old connection, goes offline, a network error, then the new hello.
	r, err := waitOnline(seq(stale, offline, adminRobot{}, fresh), old, time.Second, time.Millisecond)
	if err != nil || r.Script != "0.1.0" {
		t.Errorf("fresh hello: %+v, %v", r, err)
	}

	_, err = waitOnline(seq(stale), old, 20*time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not connect within") {
		t.Errorf("stale only: %v", err)
	}

	wrong := fresh
	wrong.Accepted = false
	_, err = waitOnline(seq(wrong), old, time.Second, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Errorf("unsupported protocol: %v", err)
	}

	// A robot that never connected before has no hello time: any hello counts.
	if _, err := waitOnline(seq(fresh), time.Time{}, time.Second, time.Millisecond); err != nil {
		t.Errorf("first connect: %v", err)
	}
}
