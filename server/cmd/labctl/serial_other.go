//go:build !darwin

package main

import "errors"

type serialPort struct{}

func openSerial(string) (*serialPort, error) {
	return nil, errors.New("robotupload runs on macOS only")
}

func (*serialPort) Read([]byte) (int, error)  { return 0, errors.ErrUnsupported }
func (*serialPort) Write([]byte) (int, error) { return 0, errors.ErrUnsupported }
func (*serialPort) Close() error              { return nil }

func errBusy(error) bool { return false }

func readPassword(string) (string, error) { return "", errors.ErrUnsupported }
