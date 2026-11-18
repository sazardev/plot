//go:build !windows

package main

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunBrokenPipeExitCode(t *testing.T) {
	t.Parallel()

	stream, _, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	stream.Out = errorWriter{err: syscall.EPIPE}
	code := run(nil, stream)

	assert.Equal(t, exitBrokenPipe, code)
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
