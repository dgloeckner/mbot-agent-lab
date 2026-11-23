package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// LED effects (led_effect) and the moods of say. The robot computes the
// frames (robot/src/fx.py); the server turns names, colors and speeds into
// numbers so the robot script stays small.

// effectRGB mirrors COLOR_RGB in robot/src/hw.py.
var effectRGB = map[string][3]int{
	"red": {255, 0, 0}, "orange": {255, 100, 0}, "yellow": {255, 200, 0}, "green": {0, 255, 0},
	"cyan": {0, 255, 255}, "blue": {0, 0, 255}, "purple": {160, 0, 255}, "white": {255, 255, 255},
}

var effectColorNames = []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple", "white"}

// effectSpec is an effect's period at normal speed and its default colors.
type effectSpec struct {
	periodMS int
	color    string
	color2   string
}

var effects = map[string]effectSpec{
	"breathe":     {3000, "blue", ""},
	"fade":        {800, "white", ""},
	"chase":       {1000, "blue", ""},
	"blink_left":  {600, "orange", ""},
	"blink_right": {600, "orange", ""},
	"rainbow":     {3000, "", ""},
	"gradient":    {0, "blue", "green"},
	"level":       {0, "green", ""},
	"heartbeat":   {1200, "red", ""},
	"off":         {0, "", ""},
}

var effectNames = []string{"breathe", "fade", "chase", "blink_left", "blink_right", "rainbow", "gradient", "level", "heartbeat", "off"}

var speedFactor = map[string]float64{"slow": 2, "normal": 1, "fast": 0.5}

// parseColor reads a color name or #rrggbb.
func parseColor(s string) ([3]int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if rgb, ok := effectRGB[s]; ok {
		return rgb, nil
	}
	if len(s) == 7 && s[0] == '#' {
		var rgb [3]int
		for i := range 3 {
			v, err := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
			if err != nil {
				break
			}
			rgb[i] = int(v)
			if i == 2 {
				return rgb, nil
			}
		}
	}
	return [3]int{}, fmt.Errorf("Die Farbe %q kenne ich nicht. Nimm %s oder #rrggbb.", s, strings.Join(effectColorNames, ", "))
}

// effectArgs builds the robot command for an effect. seconds 0 means until
// replaced.
func effectArgs(name, color, color2, speed string, value, seconds float64) (map[string]any, error) {
	spec, ok := effects[name]
	if !ok {
		return nil, fmt.Errorf("Den Effekt %q gibt es nicht. Es gibt: %s.", name, strings.Join(effectNames, ", "))
	}
	args := map[string]any{"fx": name}
	if color == "" {
		color = spec.color
	}
	if color != "" {
		rgb, err := parseColor(color)
		if err != nil {
			return nil, err
		}
		args["c"] = rgb
	}
	if color2 == "" {
		color2 = spec.color2
	}
	if name == "gradient" && color2 != "" {
		rgb, err := parseColor(color2)
		if err != nil {
			return nil, err
		}
		args["c2"] = rgb
	}
	if spec.periodMS > 0 {
		f, ok := speedFactor[speed]
		if !ok {
			f = 1
		}
		args["p"] = int(float64(spec.periodMS) * f)
	}
	if name == "level" {
		args["v"] = int(max(0, min(100, value)))
	}
	if seconds > 0 {
		args["ms"] = int(min(60, seconds) * 1000)
	}
	return args, nil
}

// moods maps say's emotion to an effect for a few seconds. The eyes and a
// gesture from the handoff's table come later.
var moods = map[string]struct{ name, color, color2, speed string }{
	"fröhlich":  {"rainbow", "", "", "fast"},
	"traurig":   {"breathe", "blue", "", "slow"},
	"genervt":   {"heartbeat", "red", "", "normal"},
	"neugierig": {"chase", "yellow", "", "normal"},
	"stolz":     {"gradient", "green", "white", "normal"},
}

var moodNames = []string{"fröhlich", "traurig", "genervt", "neugierig", "stolz"}

const moodSeconds = 5
