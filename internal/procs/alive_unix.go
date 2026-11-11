//go:build unix

package procs

import (
	"errors"
	"os"
	"syscall"
)

// signalZero probes a process without affecting it: EPERM still means it exists.
func signalZero(p *os.Process) bool {
	err := p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
