//go:build !windows

package singleinstance

import (
	"os"
	"syscall"
)

func lockProcess(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return ErrLocked
	}
	return err
}
