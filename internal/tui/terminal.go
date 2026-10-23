package tui

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// detectedKittyImage is populated once, before Bubble Tea starts reading input.
var detectedKittyImage *bool

// DetectTerminal queries Kitty graphics support and cell pixel dimensions.
// Call before starting the TUI so the query replies do not enter its key loop.
func DetectTerminal(input *os.File, output io.Writer) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CUE_IMAGES")), "ascii") {
		return
	}
	if input == nil || output == nil || !term.IsTerminal(int(input.Fd())) {
		return
	}
	oldState, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return
	}
	defer func() { _ = term.Restore(int(input.Fd()), oldState) }()
	reader, err := terminalQueryReader(input, time.Now().Add(180*time.Millisecond))
	if err != nil {
		return
	}
	defer func() {
		if reader != input {
			_ = reader.Close()
		} else {
			_ = reader.SetReadDeadline(time.Time{})
		}
	}()
	// Kitty graphics query (image id 31) plus primary DA and cell pixel size.
	if _, err := io.WriteString(output, "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c\x1b[16t"); err != nil {
		return
	}
	var buf bytes.Buffer
	chunk := make([]byte, 128)
	for buf.Len() < 2048 {
		n, err := reader.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if err != nil {
			break
		}
		if bytes.Contains(buf.Bytes(), []byte("\x1b[")) && bytes.Contains(buf.Bytes(), []byte("c")) && bytes.Contains(buf.Bytes(), []byte("t")) {
			break
		}
	}
	data := buf.Bytes()
	ok := bytes.Contains(data, []byte("\x1b_Gi=31;OK\x1b\\"))
	detectedKittyImage = &ok
	// CSI 6;height;width t is the response to CSI 16 t.
	if i := bytes.Index(data, []byte("\x1b[6;")); i >= 0 {
		end := bytes.IndexByte(data[i:], 't')
		if end > 0 {
			parts := strings.Split(string(data[i+4:i+end]), ";")
			if len(parts) == 2 {
				h, e1 := strconv.Atoi(parts[0])
				w, e2 := strconv.Atoi(parts[1])
				if e1 == nil && e2 == nil && h > 0 && w > 0 {
					kittyCellWpx, kittyCellHpx = w, h
				}
			}
		}
	}
}

// Inherited stdin can be a blocking descriptor without Go poller support.
// Reopening it gives Go a separate, pollable descriptor for the timed query.
func terminalQueryReader(input *os.File, deadline time.Time) (*os.File, error) {
	if err := input.SetReadDeadline(deadline); err == nil {
		return input, nil
	}
	reader, err := os.Open(input.Name())
	if err != nil {
		return nil, err
	}
	if err := reader.SetReadDeadline(deadline); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return reader, nil
}
