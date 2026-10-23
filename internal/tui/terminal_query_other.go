//go:build !unix

package tui

import (
	"io"
	"os"
	"time"
)

type terminalQueryFile struct {
	file  *os.File
	close bool
}

func (r *terminalQueryFile) Read(p []byte) (int, error) { return r.file.Read(p) }
func (r *terminalQueryFile) Close() error {
	if r.close {
		return r.file.Close()
	}
	return r.file.SetReadDeadline(time.Time{})
}

func terminalQueryReader(input *os.File, deadline time.Time) (io.ReadCloser, error) {
	if err := input.SetReadDeadline(deadline); err == nil {
		return &terminalQueryFile{file: input}, nil
	}
	reader, err := os.Open(input.Name())
	if err != nil {
		return nil, err
	}
	if err := reader.SetReadDeadline(deadline); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return &terminalQueryFile{file: reader, close: true}, nil
}
