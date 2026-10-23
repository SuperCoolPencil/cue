//go:build unix

package tui

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTerminalQueryPreservesSharedDescriptorFlags(t *testing.T) {
	for _, reply := range []bool{false, true} {
		name := "timeout"
		if reply {
			name = "response"
		}
		t.Run(name, func(t *testing.T) {
			fds := make([]int, 2)
			if err := unix.Pipe(fds); err != nil {
				t.Fatal(err)
			}
			input := os.NewFile(uintptr(fds[0]), "terminal-query-input")
			writer := os.NewFile(uintptr(fds[1]), "terminal-query-output")
			t.Cleanup(func() { _ = input.Close(); _ = writer.Close() })
			alias, err := unix.Dup(fds[0])
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Close(alias) })
			before, err := unix.FcntlInt(uintptr(alias), unix.F_GETFL, 0)
			if err != nil {
				t.Fatal(err)
			}
			if before&unix.O_NONBLOCK != 0 {
				t.Fatal("expected a blocking input descriptor")
			}
			if reply {
				if _, err := writer.Write([]byte("reply")); err != nil {
					t.Fatal(err)
				}
			}
			reader, err := terminalQueryReader(input, time.Now().Add(30*time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 128)
			n, readErr := reader.Read(buf)
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			if reply {
				if readErr != nil || string(buf[:n]) != "reply" {
					t.Fatalf("query response = %q, %v", buf[:n], readErr)
				}
			} else if !errors.Is(readErr, os.ErrDeadlineExceeded) {
				t.Fatalf("query timeout = %v", readErr)
			}
			for _, fd := range []int{fds[0], alias} {
				after, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
				if err != nil {
					t.Fatal(err)
				}
				if after != before {
					t.Fatalf("descriptor flags changed: %#x -> %#x", before, after)
				}
			}
		})
	}
}
