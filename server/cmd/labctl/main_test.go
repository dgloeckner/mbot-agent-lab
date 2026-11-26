package main

import (
	"strings"
	"testing"
)

func fakeGlob(devices ...string) func(string) ([]string, error) {
	return func(pattern string) ([]string, error) {
		var out []string
		for _, d := range devices {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(d, prefix) {
				out = append(out, d)
			}
		}
		return out, nil
	}
}

func TestChoosePort(t *testing.T) {
	cases := []struct {
		name, flag string
		devices    []string
		want       string
		errHas     string
	}{
		{"flag wins", "/dev/cu.other", []string{"/dev/cu.usbserial-1"}, "/dev/cu.other", ""},
		{"one usbserial", "", []string{"/dev/cu.Bluetooth-Incoming-Port", "/dev/cu.usbserial-110"}, "/dev/cu.usbserial-110", ""},
		{"one wch", "", []string{"/dev/cu.wchusbserial1420"}, "/dev/cu.wchusbserial1420", ""},
		{"none", "", []string{"/dev/cu.Bluetooth-Incoming-Port"}, "", "/dev/cu.Bluetooth-Incoming-Port"},
		{"several", "", []string{"/dev/cu.usbserial-2", "/dev/cu.wchusbserial1"}, "", "/dev/cu.usbserial-2\n  /dev/cu.wchusbserial1"},
	}
	for _, c := range cases {
		got, err := choosePort(c.flag, fakeGlob(c.devices...))
		if c.errHas != "" {
			if err == nil || !strings.Contains(err.Error(), c.errHas) {
				t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.errHas)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}
