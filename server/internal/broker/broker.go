// Package broker embeds the mochi-mqtt broker into the lab server.
//
// Robots log in with their robot ID as user name and a per-robot password and may
// only read and write below their own prefix lab/<robot_id>/. The server itself
// talks to the broker through mochi's inline client, which bypasses the network.
package broker

import (
	"bytes"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
)

// Options configure the embedded broker.
type Options struct {
	PlainAddr string            // e.g. ":1883"; empty disables the plain listener
	TLSAddr   string            // e.g. ":8883"; empty disables the TLS listener
	TLSConfig *tls.Config       // required when TLSAddr is set
	Robots    map[string]string // robot ID -> password
	Logger    *slog.Logger
}

// Broker wraps the mochi server.
type Broker struct {
	*mqtt.Server
}

// New creates the broker with authentication and listeners, but does not start it.
func New(opts Options) (*Broker, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := mqtt.New(&mqtt.Options{
		InlineClient: true,
		Logger:       logger.With("component", "mqtt"),
	})
	if err := s.AddHook(&authHook{robots: opts.Robots, log: logger}, nil); err != nil {
		return nil, err
	}
	if opts.PlainAddr != "" {
		if err := s.AddListener(listeners.NewTCP(listeners.Config{ID: "plain", Address: opts.PlainAddr})); err != nil {
			return nil, err
		}
	}
	if opts.TLSAddr != "" {
		if opts.TLSConfig == nil {
			return nil, fmt.Errorf("broker: TLS listener needs a TLS config")
		}
		if err := s.AddListener(listeners.NewTCP(listeners.Config{ID: "tls", Address: opts.TLSAddr, TLSConfig: opts.TLSConfig})); err != nil {
			return nil, err
		}
	}
	return &Broker{Server: s}, nil
}

// Handle subscribes the inline client to filter and calls fn for each message.
func (b *Broker) Handle(filter string, subID int, fn func(topic string, payload []byte)) error {
	return b.Server.Subscribe(filter, subID, func(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
		fn(pk.TopicName, pk.Payload)
	})
}

// authHook authenticates robots and restricts them to their own topic prefix.
type authHook struct {
	mqtt.HookBase
	robots map[string]string
	log    *slog.Logger
}

func (h *authHook) ID() string { return "lab-auth" }

func (h *authHook) Provides(b byte) bool {
	return bytes.Contains([]byte{mqtt.OnConnectAuthenticate, mqtt.OnACLCheck}, []byte{b})
}

func (h *authHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	user := string(pk.Connect.Username)
	want, ok := h.robots[user]
	if !ok || subtle.ConstantTimeCompare([]byte(want), pk.Connect.Password) != 1 {
		h.log.Warn("mqtt login rejected", "user", user, "remote", cl.Net.Remote)
		return false
	}
	return true
}

func (h *authHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if cl.Net.Inline {
		return true
	}
	return AllowedTopic(string(cl.Properties.Username), topic)
}

// AllowedTopic reports whether robot may use topic (or topic filter).
// Wildcards are only allowed inside the robot's own prefix.
func AllowedTopic(robotID, topic string) bool {
	if robotID == "" {
		return false
	}
	prefix := protocol.TopicPrefix + "/" + robotID + "/"
	if !strings.HasPrefix(topic, prefix) {
		return false
	}
	return !strings.ContainsAny(topic[len(prefix):], "/+#") || topic[len(prefix):] == "#"
}
