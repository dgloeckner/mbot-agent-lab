package web

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

func TestAdminRobotSetup(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := do(t, srv, "GET", "/admin/api/robots/mbot-1/setup", ""); code != http.StatusUnauthorized {
		t.Fatalf("setup without login: %d", code)
	}
	c := adminClient(t, srv)
	if code := adminDo(t, c, srv, "GET", "/admin/api/robots/mbot-1/setup", ""); code != http.StatusConflict {
		t.Errorf("without robot_setup: %d, want 409", code)
	}

	cfg := srv.lab.Config
	cfg.RobotSetup = &config.RobotSetup{WiFiSSID: "lab-guest", WiFiPassword: "wlan", BrokerHost: "mqtt.example.org", BrokerPort: 8883, BrokerTLS: true}
	cfg.Robots[1].Password = "geheim-2"
	if code := adminDo(t, c, srv, "GET", "/admin/api/robots/mbot-9/setup", ""); code != http.StatusNotFound {
		t.Errorf("unknown robot: %d, want 404", code)
	}

	resp, err := c.Get(srv.URL + "/admin/api/robots/mbot-2/setup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, Cache-Control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	var got config.RobotScriptConfig
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := config.RobotScriptConfig{RobotID: "mbot-2", WiFiSSID: "lab-guest", WiFiPassword: "wlan",
		BrokerHost: "mqtt.example.org", BrokerPort: 8883, BrokerTLS: true, BrokerPassword: "geheim-2"}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestAdminStateHasHelloTime(t *testing.T) {
	srv := newTestServerBus(t, &robotBus{})
	c := adminClient(t, srv)
	resp, err := c.Get(srv.URL + "/admin/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st struct {
		Robots []struct {
			ID      string    `json:"id"`
			HelloAt time.Time `json:"hello_at"`
		} `json:"robots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	for _, r := range st.Robots {
		if r.HelloAt.IsZero() || time.Since(r.HelloAt) > time.Minute {
			t.Errorf("%s: hello_at %v", r.ID, r.HelloAt)
		}
	}
}
