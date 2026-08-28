package player

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// mpvConn handles JSON-RPC communication with mpv over its IPC channel.
// The transport is a Unix domain socket on macOS/Linux and a named pipe on
// Windows; see mpvipc_unix.go / mpvipc_windows.go for the platform-specific
// dial logic and path helpers.
//
// A single background goroutine (readLoop) owns the decoder and dispatches
// every incoming message: command responses are delivered to their pending
// caller via a channel keyed by request id, while unsolicited events are
// handed to an optional event handler. This avoids concurrent reads on the
// shared decoder.
type mpvConn struct {
	conn    net.Conn
	enc     *json.Encoder
	dec     *json.Decoder
	writeMu sync.Mutex

	readDone  chan struct{}
	pendingMu sync.Mutex
	pending   map[int64]chan mpvResponse
	idSeq     int64

	eventMu      sync.RWMutex
	eventHandler func(event string, data json.RawMessage)
}

// mpvResponse carries the raw result of a command request.
type mpvResponse struct {
	data json.RawMessage
	err  error
}

// initMPVConn finalises a freshly dialed connection and starts its reader.
func initMPVConn(conn net.Conn, enc *json.Encoder, dec *json.Decoder) *mpvConn {
	c := &mpvConn{
		conn:     conn,
		enc:      enc,
		dec:      dec,
		readDone: make(chan struct{}),
		pending:  make(map[int64]chan mpvResponse),
	}
	go c.readLoop()
	return c
}

// readLoop owns the decoder and dispatches all incoming messages.
func (c *mpvConn) readLoop() {
	defer close(c.readDone)
	for {
		var msg struct {
			Data      json.RawMessage `json:"data"`
			Error     string          `json:"error"`
			RequestID int64           `json:"request_id"`
			Event     string          `json:"event"`
		}
		if err := c.dec.Decode(&msg); err != nil {
			c.failAll(fmt.Errorf("mpv read error: %w", err))
			return
		}

		if msg.Event != "" {
			c.eventMu.RLock()
			h := c.eventHandler
			c.eventMu.RUnlock()
			if h != nil {
				h(msg.Event, msg.Data)
			}
			continue
		}

		if msg.RequestID != 0 {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.RequestID]
			delete(c.pending, msg.RequestID)
			c.pendingMu.Unlock()
			if ok {
				var rerr error
				if msg.Error != "" && msg.Error != "success" {
					rerr = fmt.Errorf("mpv error: %s", msg.Error)
				}
				ch <- mpvResponse{data: msg.Data, err: rerr}
			}
		}
	}
}

// failAll delivers an error to every pending request (used on disconnect).
func (c *mpvConn) failAll(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		ch <- mpvResponse{err: err}
		delete(c.pending, id)
	}
}

// request sends a command and waits for its response.
func (c *mpvConn) request(command []interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.idSeq, 1)
	req := map[string]interface{}{"command": command, "request_id": id}

	ch := make(chan mpvResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := c.enc.Encode(req)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp.data, resp.err
	case <-c.readDone:
		return nil, fmt.Errorf("mpv connection closed")
	}
}

// Observe subscribes to property-change events for the named property.
func (c *mpvConn) Observe(id int64, property string) error {
	_, err := c.request([]interface{}{"observe_property", id, property})
	return err
}

// SetEventHandler registers the callback invoked for unsolicited mpv events
// (e.g. property-change, seek, end-file). The handler must not issue mpv
// requests (it runs on the reader goroutine); use the already-provided data.
func (c *mpvConn) SetEventHandler(h func(event string, data json.RawMessage)) {
	c.eventMu.Lock()
	c.eventHandler = h
	c.eventMu.Unlock()
}

// GetTimePos queries the current playback position in seconds.
func (c *mpvConn) GetTimePos() (float64, error) {
	raw, err := c.request([]interface{}{"get_property", "time-pos"})
	if err != nil {
		return 0, err
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("mpv error: %s", err)
	}
	return v, nil
}

// GetProperty queries a generic property from mpv.
func (c *mpvConn) GetProperty(name string) (interface{}, error) {
	raw, err := c.request([]interface{}{"get_property", name})
	if err != nil {
		return nil, err
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("mpv error: %s", err)
	}
	return v, nil
}

// GetPath returns the current file path/URL being played.
func (c *mpvConn) GetPath() (string, error) {
	val, err := c.GetProperty("path")
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("path is not a string")
	}
	return s, nil
}

func (c *mpvConn) Close() error {
	return c.conn.Close()
}
