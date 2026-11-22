package broker

import "testing"

func TestAllowedTopic(t *testing.T) {
	cases := []struct {
		robot, topic string
		want         bool
	}{
		{"mbot-1", "lab/mbot-1/state", true},
		{"mbot-1", "lab/mbot-1/cmd", true},
		{"mbot-1", "lab/mbot-1/#", true},
		{"mbot-1", "lab/mbot-2/cmd", false},
		{"mbot-1", "lab/mbot-10/cmd", false},
		{"mbot-1", "lab/+/cmd", false},
		{"mbot-1", "lab/#", false},
		{"mbot-1", "#", false},
		{"mbot-1", "lab/mbot-1/a/b", false},
		{"", "lab//cmd", false},
	}
	for _, c := range cases {
		if got := AllowedTopic(c.robot, c.topic); got != c.want {
			t.Errorf("AllowedTopic(%q, %q) = %v, want %v", c.robot, c.topic, got, c.want)
		}
	}
}
