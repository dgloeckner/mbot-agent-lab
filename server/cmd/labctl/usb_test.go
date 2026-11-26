package main

import (
	"bytes"
	"strings"
	"testing"
)

// ioregFixture has the shape of `ioreg -a -r -l -c IOUSBHostDevice` with a USB serial adapter
// (serial port two levels down) and a device without a serial port.
const ioregFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<array>
	<dict>
		<key>IOObjectClass</key><string>IOUSBHostDevice</string>
		<key>USB Product Name</key><string>USB Serial</string>
		<key>USB Vendor Name</key><string>QinHeng</string>
		<key>USB Serial Number</key><string>5A7B</string>
		<key>idVendor</key><integer>6790</integer>
		<key>idProduct</key><integer>29987</integer>
		<key>sessionID</key><integer>18446744072635809794</integer>
		<key>Built-In</key><false/>
		<key>IORegistryEntryChildren</key>
		<array>
			<dict>
				<key>IOObjectClass</key><string>IOUSBHostInterface</string>
				<key>IORegistryEntryChildren</key>
				<array>
					<dict>
						<key>IOObjectClass</key><string>IOSerialBSDClient</string>
						<key>IOCalloutDevice</key><string>/dev/cu.usbserial-110</string>
						<key>IODialinDevice</key><string>/dev/tty.usbserial-110</string>
					</dict>
				</array>
			</dict>
		</array>
	</dict>
	<dict>
		<key>kUSBProductString</key><string>Keyboard</string>
		<key>idVendor</key><integer>1452</integer>
		<key>idProduct</key><integer>591</integer>
		<key>Data</key><data>AAEC</data>
	</dict>
</array>
</plist>
`

func TestParseIOReg(t *testing.T) {
	devs, err := parseIOReg([]byte(ioregFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("%d devices: %+v", len(devs), devs)
	}
	d := devs[0]
	if d.Product != "USB Serial" || d.Vendor != "QinHeng" || d.Serial != "5A7B" ||
		d.VendorID != 0x1a86 || d.ProductID != 0x7523 || len(d.Ports) != 1 || d.Ports[0] != "/dev/cu.usbserial-110" {
		t.Errorf("serial adapter: %+v", d)
	}
	if k := devs[1]; k.Product != "Keyboard" || len(k.Ports) != 0 || k.VendorID != 0x05ac {
		t.Errorf("keyboard: %+v", k)
	}
	if devs, err := parseIOReg([]byte("\n")); err != nil || devs != nil {
		t.Errorf("no devices: %v, %v", devs, err)
	}
}

func TestWriteUSB(t *testing.T) {
	devs, _ := parseIOReg([]byte(ioregFixture))
	var b bytes.Buffer
	writeUSB(&b, devs, []string{"/dev/cu.Bluetooth-Incoming-Port", "/dev/cu.usbserial-110"},
		"/dev/cu.usbserial-110", map[string]string{"/dev/cu.usbserial-110": "mBlock (pid 812)"})
	want := `QinHeng USB Serial  1a86:7523  serial 5A7B
    /dev/cu.usbserial-110  <- robot:flash and robot:upload use this one  (in use by mBlock (pid 812))
Keyboard  05ac:024f
    no serial port
Other serial ports (not USB):
    /dev/cu.Bluetooth-Incoming-Port
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}

	b.Reset()
	writeUSB(&b, nil, nil, "", nil)
	if !strings.Contains(b.String(), "No USB devices found") || !strings.Contains(b.String(), "pass --port") {
		t.Errorf("empty: %s", b.String())
	}
}
