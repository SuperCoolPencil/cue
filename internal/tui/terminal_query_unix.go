//go:build unix

package tui

import (
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// terminalQueryReader waits for input without changing the terminal's file
// status flags. In particular, reopening /dev/stdin and setting a Go deadline
// can set O_NONBLOCK on the shared descriptor on macOS.
func terminalQueryReader(input *os.File, deadline time.Time) (io.ReadCloser, error) {
	return &terminalPollReader{input: input, deadline: deadline}, nil
}

type terminalPollReader struct {
	input    *os.File
	deadline time.Time
}

func (r *terminalPollReader) Read(p []byte) (int, error) {
	for {
		remaining := time.Until(r.deadline)
		if remaining <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		ms := int((remaining + time.Millisecond - 1) / time.Millisecond)
		fds := []unix.PollFd{{Fd: int32(r.input.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, os.ErrDeadlineExceeded
		}
		return r.input.Read(p)
	}
}

func (r *terminalPollReader) Close() error { return nil }
