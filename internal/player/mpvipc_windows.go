//go:build windows

package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Microsoft/go-winio"
)

// newMPVSocketPath returns a fresh Windows named-pipe path for an mpv IPC
// server. mpv on Windows uses named pipes for `--input-ipc-server`; the
// `\\.\pipe\` prefix is the kernel object namespace.
func newMPVSocketPath() string {
	return fmt.Sprintf(`\\.\pipe\cue-mpv-%d`, time.Now().UnixNano())
}

// removeMPVSocket is a no-op on Windows — named pipes are kernel objects that
// disappear once no handles reference them.
func removeMPVSocket(string) {}

// dialMPV connects to the mpv IPC named pipe. winio.DialPipe handles the
// retry/wait window via its timeout argument.
func dialMPV(pipePath string) (*mpvConn, error) {
	timeout := 3 * time.Second
	conn, err := winio.DialPipe(pipePath, &timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mpv IPC: %w", err)
	}

	return initMPVConn(conn, json.NewEncoder(conn), json.NewDecoder(bufio.NewReader(conn))), nil
}
