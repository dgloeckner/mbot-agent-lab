package cyberpi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePort plays the CyberPi: it parses written frames and queues scripted replies.
type fakePort struct {
	mu      sync.Mutex
	in      bytes.Buffer // bytes for the uploader to read
	r       Reader
	written [][]byte // bodies of all frames written, in order
	// reply returns the bytes to send back for the n-th write (0-based) of body, or nil.
	reply func(body []byte, n int) []byte
}

func newFakePort(boot string) *fakePort {
	p := &fakePort{reply: ack}
	p.in.WriteString(boot)
	return p
}

// ack answers like the firmware does.
func ack(body []byte, _ int) []byte {
	switch {
	case IsPingReply(body):
		return Frame(body)
	case body[0] == 0x28:
		ret := []byte(`{"ret":None}`)
		b := []byte{0x28, 0x04, 0x00, 0x00}
		b = binary.LittleEndian.AppendUint16(b, uint16(len(ret)))
		return Frame(append(b, ret...))
	case bytes.HasPrefix(body, []byte{0x01, 0x00, 0x5E}):
		return Frame([]byte{0x01, 0x00, 0x5E, 0xF0, 0x01, 0x00, 0x00})
	}
	return nil
}

func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, body := range p.r.Push(b) {
		n := 0
		for _, w := range p.written {
			if bytes.Equal(w, body) {
				n++
			}
		}
		p.written = append(p.written, body)
		p.in.Write(p.reply(body, n))
	}
	return len(b), nil
}

func (p *fakePort) Read(b []byte) (int, error) {
	p.mu.Lock()
	n, _ := p.in.Read(b)
	p.mu.Unlock()
	if n == 0 {
		time.Sleep(time.Millisecond) // like a serial read timeout, but short
	}
	return n, nil
}

func testOptions() Options {
	return Options{AckTimeout: 50 * time.Millisecond, Retries: 3}
}

func TestUploadSendsMBlockSequence(t *testing.T) {
	src := []byte(strings.Repeat("print('hi')\n", 20)) // 240 bytes, padded to 244
	port := newFakePort("boot log\n")
	var out bytes.Buffer
	var progress []int
	opt := testOptions()
	opt.Output = func(b []byte) { out.Write(b) }
	opt.Progress = func(done, total int) { progress = append(progress, done, total) }

	// The program prints between two acks: that output must reach Output, not break the upload.
	base := port.reply
	port.reply = func(body []byte, n int) []byte {
		r := base(body, n)
		if bytes.HasPrefix(body, []byte{0x01, 0x00, 0x5E, 0x01}) {
			r = append([]byte("noise\n"), r...)
		}
		return r
	}

	res, err := Upload(port, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	data := PadForWire(src)
	if len(data) != 244 {
		t.Fatalf("padded len %d", len(data))
	}
	steps := 3*3 + 1 + 4 // preamble x3, header, blocks 0/80/160/240
	if res.Frames != steps || res.Bytes != 244 {
		t.Errorf("result %+v", res)
	}
	if len(progress) != 2*steps || progress[len(progress)-2] != steps || progress[len(progress)-1] != steps {
		t.Errorf("progress %v", progress)
	}
	if out.String() != "boot log\nnoise\n" {
		t.Errorf("output %q", out.String())
	}

	var want [][]byte
	pre := [][]byte{Ping, ScriptImportConfig, ScriptReplOff}
	want = append(want, pre...)
	want = append(want, pre...)
	want = append(want, FileHeaderFrame(data))
	for _, b := range Blocks(data) {
		want = append(want, FileBlockFrame(b.Offset, b.Data))
	}
	want = append(want, pre...)
	if len(port.written) != len(want) {
		t.Fatalf("%d frames written, want %d", len(port.written), len(want))
	}
	for i := range want {
		if !bytes.Equal(Frame(port.written[i]), want[i]) {
			t.Errorf("frame %d:\n got % x\nwant % x", i, Frame(port.written[i]), want[i])
		}
	}

	// The CyberPi side can rebuild the file from header and blocks.
	hdr := port.written[6][6:]
	size := binary.LittleEndian.Uint32(hdr[1:5])
	sum := binary.LittleEndian.Uint32(hdr[5:9])
	if string(hdr[9:]) != StagingPath {
		t.Errorf("path %q", hdr[9:])
	}
	file := make([]byte, size)
	for _, body := range port.written[7 : 7+4] {
		p := body[6:]
		copy(file[binary.LittleEndian.Uint32(p[:4]):], p[4:])
	}
	if !bytes.Equal(file, data) || FileChecksum(file) != sum {
		t.Errorf("rebuilt file differs or checksum %#x mismatches", sum)
	}
}

func TestUploadRetriesDroppedAck(t *testing.T) {
	src := bytes.Repeat([]byte("x"), 200)
	port := newFakePort("")
	block80 := FileBlockFrame(80, PadForWire(src)[80:160])
	port.reply = func(body []byte, n int) []byte {
		if bytes.Equal(Frame(body), block80) && n == 0 {
			return nil // first ack for this block gets lost
		}
		return ack(body, n)
	}
	var logs []string
	opt := testOptions()
	opt.Log = func(s string) { logs = append(logs, s) }

	if _, err := Upload(port, src, opt); err != nil {
		t.Fatal(err)
	}
	sent := 0
	for _, w := range port.written {
		if bytes.Equal(Frame(w), block80) {
			sent++
		}
	}
	if sent != 2 {
		t.Errorf("block at 80 sent %d times, want 2", sent)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "block at 80: no reply (attempt 1 of 3)") {
		t.Errorf("logs %q", logs)
	}
}

func TestUploadFailsWhenCyberPiStaysSilent(t *testing.T) {
	port := newFakePort("")
	port.reply = func(body []byte, n int) []byte {
		if bytes.HasPrefix(body, []byte{0x01, 0x00, 0x5E, 0x01}) {
			return nil // never acknowledges the file header
		}
		return ack(body, n)
	}
	start := time.Now()
	res, err := Upload(port, []byte("print(1)\n"), testOptions())
	var nr *NoReplyError
	if !errors.As(err, &nr) || nr.Step != "file header" || nr.Attempts != 3 {
		t.Fatalf("err = %v", err)
	}
	if res.Frames != 6 {
		t.Errorf("frames before failure %d, want 6", res.Frames)
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("gave up after %s, want 3 attempts of 50ms", d)
	}
	if last := port.written[len(port.written)-1]; !bytes.HasPrefix(last, []byte{0x01, 0x00, 0x5E, 0x01}) || len(port.written) != 9 {
		t.Errorf("written %d frames, last % x", len(port.written), last)
	}
}

func TestUploadRejectsEmptyProgram(t *testing.T) {
	if _, err := Upload(newFakePort(""), nil, testOptions()); err == nil {
		t.Fatal("want error")
	}
}
