//go:build !windows

package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// newMPVSocketPath returns a fresh Unix-domain-socket path for an mpv IPC server.
func newMPVSocketPath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("cue-mpv-%d.sock", time.Now().UnixNano()))
}

// removeMPVSocket cleans up the socket file once playback ends.
func removeMPVSocket(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// dialMPV connects to the mpv IPC socket, retrying for up to 3 seconds while
// mpv finishes creating it.
func dialMPV(socketPath string) (*mpvConn, error) {
	var conn net.Conn
	var err error

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to mpv IPC: %w", err)
	}

	return initMPVConn(conn, json.NewEncoder(conn), json.NewDecoder(bufio.NewReader(conn))), nil
}
