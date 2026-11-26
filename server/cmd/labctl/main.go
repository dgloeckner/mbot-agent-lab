// Command labctl prepares the lab from the command line. macOS only for the USB commands.
//
//	labctl robot flash [--server URL] [--port DEV] [--wait 90s] ROBOT_ID
//	labctl robot upload [--port DEV] FILE
//	labctl usb
//
// flash gets the robot's setup (Wi-Fi, broker, password) from the lab server's admin API,
// bundles robot/src with it, uploads the bundle over USB and waits until the robot is online.
// upload sends a file as it is, e.g. robot/tools/hwcheck.py. usb lists USB devices and their
// serial ports, which one flash and upload would pick, and who holds a port open.
//
// Run it through Nx: npx nx run robot:flash --robot=mbot-1 --server=https://lab.example.org
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usage = `usage:
  labctl robot flash [--server URL] [--port DEV] [--wait 90s] ROBOT_ID
      Bundle robot/src with the robot's setup from the lab server, upload it over USB and wait
      until the robot is online. Admin password from LAB_ADMIN_PASSWORD or asked for.
  labctl robot upload [--port DEV] FILE
      Upload FILE as it is (e.g. robot/tools/hwcheck.py) and restart the CyberPi.
  labctl usb
      List USB devices with their serial ports, the port flash and upload pick, and which
      program holds a port open (e.g. mBlock).
`

func main() {
	cmd := strings.Join(os.Args[1:min(3, len(os.Args))], " ")
	var err error
	switch {
	case cmd == "robot flash":
		err = flashCmd(os.Args[3:])
	case cmd == "robot upload":
		err = uploadCmd(os.Args[3:])
	case len(os.Args) >= 2 && os.Args[1] == "usb":
		err = usbCmd(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	return fs
}

func uploadCmd(args []string) error {
	fs := newFlags("upload")
	port := fs.String("port", "", "serial device (default: the only CyberPi found)")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	src, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	return uploadAndRestart(fs.Arg(0), src, *port)
}

func flashCmd(args []string) error {
	fs := newFlags("flash")
	server := fs.String("server", "", "lab server (default "+defaultServer+")")
	port := fs.String("port", "", "serial device (default: the only CyberPi found)")
	wait := fs.Duration("wait", 90*time.Second, "how long to wait for the robot to come online")
	_ = fs.Parse(args)
	if fs.NArg() != 1 || fs.Arg(0) == "" {
		fs.Usage()
		os.Exit(2)
	}
	return flash(fs.Arg(0), *server, *port, *wait)
}
