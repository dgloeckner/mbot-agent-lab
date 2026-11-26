//go:build darwin

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// serialPort is a raw 115200 8N1 serial device without flow control. Reads return after at
// most 100 ms (VMIN 0, VTIME 1) with n == 0 when nothing arrived, as cyberpi.Port requires.
type serialPort struct{ fd int }

func openSerial(path string) (*serialPort, error) {
	// O_NONBLOCK: do not wait for carrier while opening; cleared once CLOCAL is set.
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := configure(fd); err != nil {
		unix.Close(fd)
		return nil, &os.PathError{Op: "configure", Path: path, Err: err}
	}
	return &serialPort{fd: fd}, nil
}

func configure(fd int) error {
	// Exclusive: mBlock (node-serialport) and pySerial take the same flock, so whoever comes
	// second gets EWOULDBLOCK; TIOCEXCL also makes later opens fail with EBUSY.
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		return err
	}
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR |
		unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY | unix.INPCK
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS
	// HUPCL drops DTR on close: closing and reopening the port restarts the CyberPi.
	t.Cflag |= unix.CS8 | unix.CLOCAL | unix.CREAD | unix.HUPCL
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1
	t.Ispeed = unix.B115200
	t.Ospeed = unix.B115200
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, t); err != nil {
		return err
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		return err
	}
	// Assert DTR and RTS like pySerial and mBlock do on open. ENOTTY: a pseudo terminal (tests)
	// has no modem lines.
	err = unix.IoctlSetPointerInt(fd, unix.TIOCMBIS, unix.TIOCM_DTR|unix.TIOCM_RTS)
	if errors.Is(err, unix.ENOTTY) {
		return nil
	}
	return err
}

func (p *serialPort) Read(b []byte) (int, error) {
	for {
		n, err := unix.Read(p.fd, b)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN):
			return 0, nil
		case err != nil:
			return 0, err
		}
		return n, nil
	}
}

func (p *serialPort) Write(b []byte) (int, error) {
	written := 0
	for written < len(b) {
		n, err := unix.Write(p.fd, b[written:])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

func (p *serialPort) Close() error { return unix.Close(p.fd) }

// errBusy reports whether opening failed because another program holds the port.
func errBusy(err error) bool {
	return errors.Is(err, unix.EBUSY) || errors.Is(err, unix.EWOULDBLOCK)
}
