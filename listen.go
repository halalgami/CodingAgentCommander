package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// stableListen opens Commander's loopback listener on the port it used last
// time when that port is free, else on a new one, and remembers it. Running
// sessions keep their hook and room-tool URLs across a Commander restart
// (rooms review I2).
func stableListen(path string) (net.Listener, error) {
	if b, err := os.ReadFile(path); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 1024 && p < 65536 {
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
				return ln, nil
			}
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)), 0o600)
	return ln, nil
}
