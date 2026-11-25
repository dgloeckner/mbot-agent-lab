package robots

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
)

// fakeBus answers every cmd with a configurable result, like a robot would.
type fakeBus struct {
	mu      sync.Mutex
	handler func(topic string, payload []byte)
	reply   func(cmd protocol.Cmd) *protocol.Result
}

func (b *fakeBus) Handle(_ string, _ int, fn func(string, []byte)) error {
	b.mu.Lock()
	b.handler = fn
	b.mu.Unlock()
	return nil
}

func (b *fakeBus) deliver(topic, payload string) {
	b.mu.Lock()
	h := b.handler
	b.mu.Unlock()
	h(topic, []byte(payload))
}

func (b *fakeBus) Publish(topic string, payload []byte, _ bool, _ byte) error {
	id, suffix, _ := protocol.ParseTopic(topic)
	if suffix == protocol.TopicCmd && b.reply != nil {
		var cmd protocol.Cmd
		_ = json.Unmarshal(payload, &cmd)
		if res := b.reply(cmd); res != nil {
			raw, _ := json.Marshal(res)
			go b.deliver(protocol.Topic(id, protocol.TopicResult), string(raw))
		}
	}
	return nil
}

func newOnline(t *testing.T, bus *fakeBus) *Registry {
	t.Helper()
	r := New(bus, []Spec{{ID: "mbot-1", Color: "red"}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go r.Run(ctx)
	for {
		bus.mu.Lock()
		ready := bus.handler != nil
		bus.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	bus.deliver("lab/mbot-1/status", "online")
	bus.deliver("lab/mbot-1/hello", `{"v":1,"script":"0.1.0","mac":"aa"}`)
	return r
}

func TestSendMatchesResult(t *testing.T) {
	bus := &fakeBus{reply: func(c protocol.Cmd) *protocol.Result {
		return &protocol.Result{ID: c.ID, OK: true, Reason: protocol.ReasonDone}
	}}
	r := newOnline(t, bus)
	res, err := r.Send(context.Background(), "mbot-1", "stop", nil, time.Second)
	if err != nil || !res.OK || res.Reason != protocol.ReasonDone {
		t.Fatalf("Send = %+v, %v", res, err)
	}
}

func TestSendTimeout(t *testing.T) {
	r := newOnline(t, &fakeBus{})
	res, err := r.Send(context.Background(), "mbot-1", "drive", nil, 20*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || res.Reason != protocol.ReasonTimeout {
		t.Fatalf("Send = %+v, %v", res, err)
	}
}

func TestSendRejectsOfflineAndOldVersion(t *testing.T) {
	bus := &fakeBus{}
	r := newOnline(t, bus)
	bus.deliver("lab/mbot-1/hello", `{"v":99}`)
	if _, err := r.Send(context.Background(), "mbot-1", "stop", nil, time.Second); !errors.Is(err, ErrNotAccepted) {
		t.Errorf("want ErrNotAccepted, got %v", err)
	}
	bus.deliver("lab/mbot-1/status", "offline")
	if _, err := r.Send(context.Background(), "mbot-1", "stop", nil, time.Second); !errors.Is(err, ErrOffline) {
		t.Errorf("want ErrOffline, got %v", err)
	}
	if _, err := r.Send(context.Background(), "mbot-9", "stop", nil, time.Second); !errors.Is(err, ErrUnknownRobot) {
		t.Errorf("want ErrUnknownRobot, got %v", err)
	}
}

func TestEventsAndState(t *testing.T) {
	bus := &fakeBus{}
	r := newOnline(t, bus)
	ch := make(chan Event, 1)
	r.Subscribe(ch)
	bus.deliver("lab/mbot-1/state", `{"skill":"idle","distance_cm":34,"battery":81}`)
	bus.deliver("lab/mbot-1/event", `{"kind":"obstacle_near","data":{"distance_cm":8},"seq":1,"t_ms":5}`)
	ev := <-ch
	if ev.RobotID != "mbot-1" || ev.Event.Kind != protocol.EventObstacleNear {
		t.Errorf("event = %+v", ev)
	}
	snap := r.Snapshot()
	if snap[0].State.DistanceCM != 34 || !snap[0].Online || !snap[0].Accepted {
		t.Errorf("snapshot = %+v", snap[0])
	}
}

func TestReassignEndsRunningCommand(t *testing.T) {
	r := newOnline(t, &fakeBus{})
	_ = r.Assign("mbot-1", Owner{TeamID: "rot", TeamName: "Rot", Color: "red", Alias: "Scout"})
	done := make(chan protocol.Result, 1)
	go func() {
		res, _ := r.Send(context.Background(), "mbot-1", "drive", nil, 5*time.Second)
		done <- res
	}()
	time.Sleep(20 * time.Millisecond) // let Send register the pending command
	_ = r.Assign("mbot-1", Owner{TeamID: "blau", TeamName: "Blau", Color: "blue", Alias: "rot"})
	select {
	case res := <-done:
		if res.Reason != protocol.ReasonReassigned {
			t.Errorf("reason = %q", res.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("running command did not end on reassignment")
	}
	snap := r.Snapshot()[0]
	if snap.Owner != "blau" || snap.TeamColor != "blue" {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestCommandIDsDifferAcrossRestarts(t *testing.T) {
	a := New(&fakeBus{}, nil, nil).nextCmdID()
	b := New(&fakeBus{}, nil, nil).nextCmdID()
	if a == b {
		t.Errorf("two server starts produced the same first command ID %q", a)
	}
}

func TestFreshStateWaitsForANewerReading(t *testing.T) {
	bus := &fakeBus{}
	reg := newOnline(t, bus)
	ctx := context.Background()
	bus.deliver("lab/mbot-1/state", `{"distance_cm":50,"seq":1}`)
	after := time.Now()
	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.deliver("lab/mbot-1/state", `{"distance_cm":12,"seq":2}`)
	}()
	rb, err := reg.FreshState(ctx, "mbot-1", after, time.Second)
	if err != nil || rb.State.DistanceCM != 12 {
		t.Fatalf("fresh state = %+v, %v", rb.State, err)
	}
	if _, err := reg.FreshState(ctx, "mbot-1", time.Now(), 60*time.Millisecond); !errors.Is(err, ErrNoFreshState) {
		t.Errorf("no newer state: %v", err)
	}
	if _, err := reg.FreshState(ctx, "mbot-9", after, time.Second); !errors.Is(err, ErrUnknownRobot) {
		t.Errorf("unknown robot: %v", err)
	}
}
