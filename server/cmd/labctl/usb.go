package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// usbDevice is one USB device from the IOKit registry, with the serial ports it provides.
type usbDevice struct {
	Product, Vendor, Serial string
	VendorID, ProductID     int
	Ports                   []string // /dev/cu.* call-out devices
}

func usbCmd(args []string) error {
	fs := newFlags("usb")
	_ = fs.Parse(args)
	if runtime.GOOS != "darwin" {
		return errors.New("labctl usb runs on macOS only")
	}
	out, err := exec.Command("ioreg", "-a", "-r", "-l", "-c", "IOUSBHostDevice").Output()
	if err != nil {
		return fmt.Errorf("ioreg: %w", err)
	}
	devs, err := parseIOReg(out)
	if err != nil {
		return err
	}
	all, _ := filepath.Glob("/dev/cu.*")
	picked, _ := choosePort("", filepath.Glob)
	writeUSB(os.Stdout, devs, all, picked, portUsers(all))
	return nil
}

// writeUSB prints the USB devices, their serial ports, which port flash and upload pick without
// --port, and who holds a port open.
func writeUSB(w io.Writer, devs []usbDevice, allPorts []string, picked string, users map[string]string) {
	port := func(p string) string {
		s := "    " + p
		if p == picked {
			s += "  <- robot:flash and robot:upload use this one"
		}
		if u := users[p]; u != "" {
			s += "  (in use by " + u + ")"
		}
		return s
	}
	if len(devs) == 0 {
		fmt.Fprintln(w, "No USB devices found. Plug the CyberPi in with a data cable and switch the battery on.")
	}
	seen := map[string]bool{}
	for _, d := range devs {
		name := strings.TrimSpace(d.Vendor + " " + d.Product)
		if name == "" {
			name = "(unnamed device)"
		}
		fmt.Fprintf(w, "%s  %04x:%04x", name, d.VendorID, d.ProductID)
		if d.Serial != "" {
			fmt.Fprintf(w, "  serial %s", d.Serial)
		}
		fmt.Fprintln(w)
		if len(d.Ports) == 0 {
			fmt.Fprintln(w, "    no serial port")
		}
		for _, p := range d.Ports {
			fmt.Fprintln(w, port(p))
			seen[p] = true
		}
	}
	var other []string
	for _, p := range allPorts {
		if !seen[p] {
			other = append(other, p)
		}
	}
	if len(other) > 0 {
		fmt.Fprintln(w, "Other serial ports (not USB):")
		for _, p := range other {
			fmt.Fprintln(w, port(p))
		}
	}
	if picked == "" {
		fmt.Fprintln(w, "No single CyberPi port found: pass --port=/dev/cu.… to robot:flash and robot:upload.")
	}
}

// portUsers asks lsof which programs hold the ports open, e.g. mBlock. Opening a port ourselves
// would restart the CyberPi, so we never probe that way.
func portUsers(ports []string) map[string]string {
	users := map[string]string{}
	if len(ports) == 0 {
		return users
	}
	out, _ := exec.Command("lsof", append([]string{"-F", "pcn"}, ports...)...).Output()
	var pid, cmd string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch v := line[1:]; line[0] {
		case 'p':
			pid = v
		case 'c':
			cmd = v
		case 'n':
			if slices.Contains(ports, v) {
				users[v] = cmd + " (pid " + pid + ")"
			}
		}
	}
	return users
}

// parseIOReg reads `ioreg -a -r -l -c IOUSBHostDevice`: a plist array of USB devices whose
// child entries include the IOSerialBSDClient with the device path.
func parseIOReg(raw []byte) ([]usbDevice, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil // no USB devices: ioreg prints nothing
	}
	v, err := parsePlist(raw)
	if err != nil {
		return nil, fmt.Errorf("ioreg output: %w", err)
	}
	roots, _ := v.([]any)
	var devs []usbDevice
	for _, r := range roots {
		e, ok := r.(map[string]any)
		if !ok {
			continue
		}
		str := func(k string) string { s, _ := e[k].(string); return s }
		num := func(k string) int { n, _ := e[k].(int64); return int(n) }
		d := usbDevice{
			Product: str("USB Product Name"), Vendor: str("USB Vendor Name"), Serial: str("USB Serial Number"),
			VendorID: num("idVendor"), ProductID: num("idProduct"),
		}
		if d.Product == "" {
			d.Product = str("kUSBProductString")
		}
		collectPorts(e, &d.Ports)
		slices.Sort(d.Ports)
		devs = append(devs, d)
	}
	return devs, nil
}

func collectPorts(e map[string]any, out *[]string) {
	if p, ok := e["IOCalloutDevice"].(string); ok {
		*out = append(*out, p)
	}
	children, _ := e["IORegistryEntryChildren"].([]any)
	for _, c := range children {
		if m, ok := c.(map[string]any); ok {
			collectPorts(m, out)
		}
	}
}

// parsePlist decodes an XML property list into map[string]any, []any, string, int64, float64
// and bool; data and dates come back as strings.
func parsePlist(raw []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local != "plist" {
			return plistValue(dec, se)
		}
	}
}

func plistValue(dec *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					var k string
					if err := dec.DecodeElement(&k, &t); err != nil {
						return nil, err
					}
					key = k
					continue
				}
				v, err := plistValue(dec, t)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		var a []any
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(dec, t)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			case xml.EndElement:
				return a, nil
			}
		}
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return se.Name.Local == "true", nil
	}
	var s string
	if err := dec.DecodeElement(&s, &se); err != nil {
		return nil, err
	}
	switch se.Name.Local {
	case "integer":
		if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return n, nil
		}
		return s, nil // beyond int64 (IOKit has unsigned 64-bit values): keep the text
	case "real":
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	return s, nil
}
