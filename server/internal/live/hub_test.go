package live

import "testing"

func TestBacklogAndSignalsFollowTheActiveTask(t *testing.T) {
	h := NewHub()
	h.SetTask("rot", "a1")
	h.Publish("rot", LogEntry{Kind: "deploy"})
	_, ch, cancel := h.Subscribe("rot", 0)
	defer cancel()

	h.SetTask("rot", "a2")
	if e := <-ch; e.Signal != SignalTask || e.Task != "a2" {
		t.Fatalf("signal = %+v", e)
	}
	h.SetTask("rot", "a2") // unchanged: no signal
	e := h.Publish("rot", LogEntry{Kind: "chat"})
	if e.Task != "a2" {
		t.Errorf("entry task = %q", e.Task)
	}
	if got := <-ch; got.ID != e.ID {
		t.Errorf("next on channel = %+v", got)
	}
	h.Signal("rot", SignalTasks)
	if got := <-ch; got.Signal != SignalTasks {
		t.Errorf("tasks signal = %+v", got)
	}

	backlog, _, cancel2 := h.Subscribe("rot", 0)
	defer cancel2()
	if len(backlog) != 1 || backlog[0].Kind != "chat" {
		t.Errorf("backlog after the switch = %+v", backlog)
	}
}
