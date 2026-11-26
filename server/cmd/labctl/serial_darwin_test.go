//go:build darwin

package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/cyberpi"
)

// openPTY returns the controller side and the device path of a pseudo terminal, which stands
// in for the USB serial device.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	ctl, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skip("no pty:", err)
	}
	t.Cleanup(func() { ctl.Close() })
	fd := int(ctl.Fd())
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		t.Fatal(err)
	}
	var name [128]byte // TIOCPTYGNAME fills a 128-byte buffer
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0])))
	if errno != 0 {
		t.Fatal(errno)
	}
	return ctl, strings.TrimRight(string(name[:]), "\x00")
}

func TestSerialPortOnPTY(t *testing.T) {
	ctl, path := openPTY(t)
	p, err := openSerial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// A second open fails: the port is exclusive.
	if _, err := openSerial(path); err == nil || !errBusy(err) {
		t.Errorf("second open: %v, want busy", err)
	}

	// Raw mode: bytes pass unchanged, both ways.
	frame := []byte{0xF3, 0xF6, 0x03, 0x00, 0x0D, 0x0A, 0x00, 0x0D, 0xF4}
	if _, err := p.Write(frame); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 64)
	n, err := ctl.Read(got)
	if err != nil || !bytes.Equal(got[:n], frame) {
		t.Fatalf("controller read % x, %v", got[:n], err)
	}
	if _, err := ctl.Write(frame); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	n, err = p.Read(got)
	if err != nil || !bytes.Equal(got[:n], frame) {
		t.Fatalf("port read % x, %v", got[:n], err)
	}

	// Nothing to read: returns 0 after the VTIME timeout instead of blocking.
	start := time.Now()
	n, err = p.Read(got)
	if n != 0 || err != nil {
		t.Fatalf("idle read %d, %v", n, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("idle read took %s", d)
	}
}

// TestUploadOverPTY runs a full upload through the real serial code against a fake CyberPi on
// the controller side of a pseudo terminal.
func TestUploadOverPTY(t *testing.T) {
	ctl, path := openPTY(t)
	src := []byte(strings.Repeat("print('hello')\n", 30))
	data := cyberpi.PadForWire(src)
	got := make([]byte, len(data))
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := cyberpi.Reader{}
		buf := make([]byte, 4096)
		for {
			n, err := ctl.Read(buf)
			if err != nil {
				return
			}
			for _, body := range r.Push(buf[:n]) {
				var reply []byte
				switch {
				case cyberpi.IsPingReply(body):
					reply = body
				case body[0] == 0x28:
					reply = []byte("\x28\x04\x00\x00\x0c\x00{\"ret\":None}")
				case body[3] == 0x02:
					off := int(body[6]) | int(body[7])<<8 | int(body[8])<<16 | int(body[9])<<24
					copy(got[off:], body[10:])
					fallthrough
				default:
					reply = []byte{0x01, 0x00, 0x5E, 0xF0, 0x01, 0x00, 0x00}
				}
				ctl.Write(append([]byte("log line\n"), cyberpi.Frame(reply)...))
			}
		}
	}()

	p, err := openSerial(path)
	if err != nil {
		t.Fatal(err)
	}
	opt := cyberpi.DefaultOptions()
	opt.BootWait = 0
	opt.AckTimeout = time.Second
	res, err := cyberpi.Upload(p, src, opt)
	p.Close()
	ctl.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if res.Bytes != len(data) || !bytes.Equal(got, data) {
		t.Errorf("fake CyberPi received a different file (result %+v)", res)
	}
}

func TestPortUsersNamesTheHolder(t *testing.T) {
	_, path := openPTY(t)
	p, err := openSerial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	users := portUsers([]string{path})
	if u := users[path]; !strings.Contains(u, "(pid "+strconv.Itoa(os.Getpid())+")") {
		t.Errorf("users = %v", users)
	}
}
