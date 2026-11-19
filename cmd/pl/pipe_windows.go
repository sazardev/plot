//go:build windows

package main

import (
	"errors"
	"syscall"
)

func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE)
}
