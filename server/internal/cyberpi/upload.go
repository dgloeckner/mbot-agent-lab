package cyberpi

// Upload sequence ported from PyOpenBlock's uploader.py (MIT, see the notice in protocol.go).

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// Port is an open serial connection. Read must return within a short time (about 100 ms) with
// n == 0 and a nil error when nothing arrived, so ack timeouts can be enforced.
type Port interface {
	io.Reader
	io.Writer
}

// Options tune an upload. Zero values are not useful; start from DefaultOptions.
type Options struct {
	BootWait   time.Duration // after opening the port: opening it restarts the CyberPi
	AckTimeout time.Duration // per attempt
	Retries    int           // attempts per frame

	Output   func([]byte)          // program output (non-frame bytes); may be nil
	Log      func(string)          // status messages; may be nil
	Progress func(done, total int) // after every acknowledged frame; may be nil
}

// DefaultOptions match mBlock and the reference uploader.
func DefaultOptions() Options {
	return Options{BootWait: 3 * time.Second, AckTimeout: 3 * time.Second, Retries: 3}
}

// NoReplyError means the CyberPi did not acknowledge a frame after all attempts.
type NoReplyError struct {
	Step     string
	Attempts int
}

func (e *NoReplyError) Error() string {
	return fmt.Sprintf("no reply from the CyberPi to %s after %d attempts", e.Step, e.Attempts)
}

// Result summarises a finished upload.
type Result struct {
	Bytes  int // bytes on the wire, padding included
	Frames int // acknowledged frames
}

type step struct {
	frame []byte
	match func([]byte) bool
	label string
}

// plan returns the frames of an upload in order, as mBlock sends them: the preamble twice,
// the file header, the blocks, the preamble once more.
func plan(data []byte) []step {
	preamble := func(pass string) []step {
		return []step{
			{Ping, IsPingReply, "upload mode (" + pass + ")"},
			{ScriptImportConfig, IsScriptReply, "import config (" + pass + ")"},
			{ScriptReplOff, IsScriptReply, "repl off (" + pass + ")"},
		}
	}
	steps := append(preamble("1"), preamble("2")...)
	steps = append(steps, step{FileHeaderFrame(data), IsFileAck, "file header"})
	for _, b := range Blocks(data) {
		steps = append(steps, step{FileBlockFrame(b.Offset, b.Data), IsFileAck, fmt.Sprintf("block at %d", b.Offset)})
	}
	return append(steps, preamble("finish")...)
}

// Upload writes src to the CyberPi's staging slot. The caller restarts the CyberPi afterwards
// (close and reopen the port) so the firmware moves the file into place and runs it.
func Upload(port Port, src []byte, opt Options) (Result, error) {
	if len(src) == 0 {
		return Result{}, errors.New("empty program")
	}
	if opt.Retries < 1 || opt.AckTimeout <= 0 {
		return Result{}, errors.New("cyberpi: invalid options")
	}
	logf := func(format string, a ...any) {
		if opt.Log != nil {
			opt.Log(fmt.Sprintf(format, a...))
		}
	}
	data := PadForWire(src)
	steps := plan(data)
	r := &Reader{Text: opt.Output}
	buf := make([]byte, 4096)

	if opt.BootWait > 0 {
		logf("Waiting %s for the CyberPi to boot ...", opt.BootWait)
		time.Sleep(opt.BootWait)
	}
	// Drop boot output and stray frames that arrived before anyone waited for them.
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		n, err := port.Read(buf)
		if err != nil {
			return Result{}, fmt.Errorf("read: %w", err)
		}
		if n == 0 {
			break
		}
		r.Push(buf[:n])
	}

	for i, s := range steps {
		if err := transact(port, r, buf, s, opt, logf); err != nil {
			return Result{Bytes: len(data), Frames: i}, err
		}
		if opt.Progress != nil {
			opt.Progress(i+1, len(steps))
		}
	}
	return Result{Bytes: len(data), Frames: len(steps)}, nil
}

func transact(port Port, r *Reader, buf []byte, s step, opt Options, logf func(string, ...any)) error {
	for attempt := 1; attempt <= opt.Retries; attempt++ {
		if _, err := port.Write(s.frame); err != nil {
			return fmt.Errorf("write %s: %w", s.label, err)
		}
		for deadline := time.Now().Add(opt.AckTimeout); time.Now().Before(deadline); {
			n, err := port.Read(buf)
			if err != nil {
				return fmt.Errorf("read %s: %w", s.label, err)
			}
			for _, body := range r.Push(buf[:n]) {
				if s.match(body) {
					return nil
				}
			}
		}
		logf("%s: no reply (attempt %d of %d)", s.label, attempt, opt.Retries)
	}
	return &NoReplyError{Step: s.label, Attempts: opt.Retries}
}
