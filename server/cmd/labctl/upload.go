package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/cyberpi"
)

// Device names of the CyberPi's USB serial chip (macOS driver and WCH driver).
var portPatterns = []string{"/dev/cu.usbserial-*", "/dev/cu.wchusbserial*"}

const hints = `Check:
  - the battery switch on the mBot2 is on (on USB power alone the CyberPi does not answer)
  - mBlock is closed (it holds the serial port)
  - the USB cable is a data cable, not a charge-only cable`

// uploadAndRestart uploads src to the CyberPi on port (or the only one found) and restarts it.
// name only appears in messages.
func uploadAndRestart(name string, src []byte, port string) error {
	if len(src) == 0 {
		return fmt.Errorf("%s is empty", name)
	}
	path, err := choosePort(port, filepath.Glob)
	if err != nil {
		return err
	}
	p, err := open(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Uploading %s (%d bytes) to %s\n", name, len(src), path)
	opt := cyberpi.DefaultOptions()
	opt.Output = func(b []byte) { os.Stdout.Write(b) }
	opt.Log = func(s string) { fmt.Fprintln(os.Stderr, s) }
	last := -1
	opt.Progress = func(done, total int) {
		if pct := done * 100 / total / 10 * 10; pct != last {
			last = pct
			fmt.Fprintf(os.Stderr, "%d%%\n", pct)
		}
	}
	res, err := cyberpi.Upload(p, src, opt)
	p.Close()
	var nr *cyberpi.NoReplyError
	if errors.As(err, &nr) {
		return fmt.Errorf("%w\n%s", err, hints)
	}
	if err != nil {
		return err
	}

	// Close, wait, reopen (toggles DTR): the CyberPi restarts, moves the file into place and
	// runs it.
	fmt.Fprintln(os.Stderr, "Restarting the CyberPi ...")
	time.Sleep(300 * time.Millisecond)
	p, err = open(path)
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	p.Close()
	fmt.Fprintf(os.Stderr, "Uploaded %d bytes, the CyberPi runs the new program.\n", res.Bytes)
	return nil
}

func open(path string) (*serialPort, error) {
	p, err := openSerial(path)
	switch {
	case err == nil:
		return p, nil
	case errBusy(err):
		return nil, fmt.Errorf("%s is busy: close mBlock and any serial monitor, then try again", path)
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%s not found: is the CyberPi plugged in with a data cable and switched on?", path)
	}
	return nil, err
}

// choosePort returns port if given, else the only CyberPi-looking serial device.
func choosePort(port string, glob func(string) ([]string, error)) (string, error) {
	if port != "" {
		return port, nil
	}
	var found []string
	for _, pat := range portPatterns {
		m, _ := glob(pat)
		found = append(found, m...)
	}
	sort.Strings(found)
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		all, _ := glob("/dev/cu.*")
		msg := "no CyberPi found on USB: plug it in with a data cable and switch the battery on"
		if len(all) > 0 {
			msg += "\nserial ports: " + strings.Join(all, ", ") + "\npick one with --port"
		}
		return "", errors.New(msg + "\nnpx nx run robot:usb lists the USB devices")
	}
	return "", fmt.Errorf("several serial ports found, pick one with --port:\n  %s\nnpx nx run robot:usb shows which device each belongs to",
		strings.Join(found, "\n  "))
}
