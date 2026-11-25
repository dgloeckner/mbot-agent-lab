package web

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
)

// robotBus answers every command like a robot. With hold set, motion
// commands wait for release; stop always answers.
type robotBus struct {
	mu      sync.Mutex
	handler func(string, []byte)
	cmds    []string
	hold    bool
	held    []func()
	reply   func(cmd protocol.Cmd) protocol.Result // nil: done
}

func (b *robotBus) Handle(_ string, _ int, fn func(string, []byte)) error {
	b.mu.Lock()
	b.handler = fn
	b.mu.Unlock()
	return nil
}

func (b *robotBus) Publish(topic string, payload []byte, _ bool, _ byte) error {
	id, suffix, _ := protocol.ParseTopic(topic)
	if suffix != protocol.TopicCmd {
		return nil
	}
	var cmd protocol.Cmd
	_ = json.Unmarshal(payload, &cmd)
	b.mu.Lock()
	defer b.mu.Unlock()
	res := protocol.Result{ID: cmd.ID, OK: true, Reason: protocol.ReasonDone, Data: json.RawMessage(`{"driven_cm":20}`)}
	if b.reply != nil {
		res = b.reply(cmd)
		res.ID = cmd.ID
	}
	raw, _ := json.Marshal(res)
	h := b.handler
	b.cmds = append(b.cmds, id+":"+cmd.Name)
	answer := func() { h(protocol.Topic(id, protocol.TopicResult), raw) }
	if b.hold && cmd.Name != "stop" && cmd.Name != "clear_log" {
		b.held = append(b.held, answer)
		return nil
	}
	go answer()
	return nil
}

func (b *robotBus) sent() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.cmds...)
}

func (b *robotBus) holding() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.held)
}

func (b *robotBus) setHold(on bool) {
	b.mu.Lock()
	b.hold = on
	held := b.held
	b.held = nil
	b.mu.Unlock()
	if !on {
		for _, f := range held {
			f()
		}
	}
}

// online starts the registry on this bus and brings every robot online.
func (b *robotBus) online(t *testing.T, reg *robots.Registry) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = reg.Run(ctx) }()
	for {
		b.mu.Lock()
		h := b.handler
		b.mu.Unlock()
		if h != nil {
			for _, id := range []string{"mbot-1", "mbot-2", "mbot-3"} {
				h("lab/"+id+"/status", []byte("online"))
				h("lab/"+id+"/hello", []byte(`{"v":1}`))
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
}
