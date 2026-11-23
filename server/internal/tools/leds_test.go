package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestEffectArgs(t *testing.T) {
	cases := []struct {
		name, color, color2, speed string
		value, seconds             float64
		want                       map[string]any
	}{
		{"breathe", "", "", "", 0, 0, map[string]any{"fx": "breathe", "c": [3]int{0, 0, 255}, "p": 3000}},
		{"breathe", "#FF8000", "", "slow", 0, 90, map[string]any{"fx": "breathe", "c": [3]int{255, 128, 0}, "p": 6000, "ms": 60000}},
		{"gradient", "green", "white", "", 0, 0, map[string]any{"fx": "gradient", "c": [3]int{0, 255, 0}, "c2": [3]int{255, 255, 255}}},
		{"level", "", "", "fast", 140, 2, map[string]any{"fx": "level", "c": [3]int{0, 255, 0}, "v": 100, "ms": 2000}},
		{"rainbow", "", "", "fast", 0, 0, map[string]any{"fx": "rainbow", "p": 1500}},
		{"off", "", "", "", 0, 0, map[string]any{"fx": "off"}},
	}
	for _, tc := range cases {
		got, err := effectArgs(tc.name, tc.color, tc.color2, tc.speed, tc.value, tc.seconds)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %s: %v, %v; want %v", tc.name, tc.color, got, err, tc.want)
		}
	}
	for _, bad := range [][2]string{{"disco", "red"}, {"fade", "pink"}, {"fade", "#12345g"}, {"fade", "#123"}} {
		if _, err := effectArgs(bad[0], bad[1], "", "", 0, 0); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
	for _, m := range moodNames {
		mood := moods[m]
		if _, err := effectArgs(mood.name, mood.color, mood.color2, mood.speed, 0, moodSeconds); err != nil {
			t.Errorf("mood %s: %v", m, err)
		}
	}
}

func TestLedEffectAndMoods(t *testing.T) {
	r, bus, _ := setup(t, map[string]string{"mbot-1": "Scout"})
	ctx := context.Background()
	env := &Env{TeamID: "rot", Agents: []string{"Blau"}, Say: func(to, text string) []string { return []string{to} }}
	got := schemaOf(t, r.Call(ctx, env, "led_effect", `{"name":"chase","color":"pink"}`))
	if got["ok"] != false || !strings.Contains(got["error"].(string), "pink") {
		t.Errorf("bad color = %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "led_effect", `{"name":"blink_left","speed":"fast","seconds":3}`))
	if got["ok"] != true {
		t.Errorf("led_effect = %v", got)
	}
	bus.mu.Lock()
	args := bus.args["led_effect"]
	bus.mu.Unlock()
	if args["fx"] != "blink_left" || args["p"] != 300.0 || args["ms"] != 3000.0 {
		t.Errorf("led_effect args = %v", args)
	}
	bus.mu.Lock()
	bus.args["led_effect"] = nil
	bus.mu.Unlock()
	got = schemaOf(t, r.Call(ctx, env, "say", `{"to":"Blau","text":"Geschafft!","emotion":"stolz"}`))
	if got["ok"] != true {
		t.Errorf("say = %v", got)
	}
	bus.mu.Lock()
	args = bus.args["led_effect"]
	bus.mu.Unlock()
	if args["fx"] != "gradient" || args["ms"] != 5000.0 {
		t.Errorf("say mood effect = %v", args)
	}
}
