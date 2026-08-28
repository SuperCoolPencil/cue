package player

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

type timelineReport struct {
	State     string
	RatingKey string
	TimeMs    int64
	Duration  int64
}

type mockPlaybackClient struct {
	domain.PlaybackClient
	marks     []string
	progress  map[string]int64
	timelines []timelineReport
	mu        sync.Mutex
}

func (m *mockPlaybackClient) MarkPlayed(ctx context.Context, itemID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marks = append(m.marks, itemID)
	return nil
}

func (m *mockPlaybackClient) UpdateProgress(ctx context.Context, itemID string, pos int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.progress == nil {
		m.progress = make(map[string]int64)
	}
	m.progress[itemID] = pos
	return nil
}

func (m *mockPlaybackClient) ReportTimeline(ctx context.Context, state, ratingKey string, timeMs, durationMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timelines = append(m.timelines, timelineReport{state, ratingKey, timeMs, durationMs})
	return nil
}

func (m *mockPlaybackClient) GetTimelines() []timelineReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]timelineReport, len(m.timelines))
	copy(out, m.timelines)
	return out
}

func (m *mockPlaybackClient) GetMarks() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.marks...)
}

func (m *mockPlaybackClient) GetProgress(id string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.progress[id]
}

func TestScrobbler(t *testing.T) {
	// Setup mock MPV server
	tmpDir, err := os.MkdirTemp("", "cue-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	socketPath := filepath.Join(tmpDir, "mpv.sock")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	// Mock server state
	serverState := struct {
		pos         float64
		playlistPos int
		mu          sync.Mutex
	}{
		pos:         10.0,
		playlistPos: 0,
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				dec := json.NewDecoder(c)
				for {
					var req struct {
						Command   []interface{} `json:"command"`
						RequestID int64         `json:"request_id"`
					}
					if err := dec.Decode(&req); err != nil {
						return
					}
					if len(req.Command) == 0 {
						continue
					}

					var resp interface{}
					cmd := req.Command[0].(string)
					switch cmd {
					case "get_property":
						prop := req.Command[1].(string)
						serverState.mu.Lock()
						switch prop {
						case "time-pos":
							resp = serverState.pos
						case "playlist-pos":
							resp = float64(serverState.playlistPos)
						}
						serverState.mu.Unlock()
					}

					res, _ := json.Marshal(map[string]interface{}{
						"data":       resp,
						"error":      "success",
						"request_id": req.RequestID,
					})
					_, _ = c.Write(append(res, '\n'))
				}
			}(conn)
		}
	}()

	client := &mockPlaybackClient{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewScrobbler(client, logger)
	s.interval = 50 * time.Millisecond // fast polling for tests

	items := []domain.MediaItem{
		{ID: "1", Title: "Ep 1", Duration: 100 * time.Second},
		{ID: "2", Title: "Ep 2", Duration: 100 * time.Second},
	}

	// Mock command that "runs" for a bit
	cmd := exec.Command("sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle := s.Monitor(ctx, cmd, socketPath, 0, 0, items...)

	// 1. Check progress reporting
	time.Sleep(200 * time.Millisecond)
	if p := client.GetProgress("1"); p == 0 {
		t.Error("expected progress update for item 1")
	}

	// 2. Change playlist position
	serverState.mu.Lock()
	serverState.playlistPos = 1
	serverState.pos = 5.0
	serverState.mu.Unlock()

	time.Sleep(200 * time.Millisecond)
	marks := client.GetMarks()
	found := false
	for _, m := range marks {
		if m == "1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected item 1 to be marked watched after playlist change")
	}

	// 3. Test auto-scrobble on exit (>90%)
	serverState.mu.Lock()
	serverState.pos = 95.0 // 95/100 = 95%
	serverState.mu.Unlock()

	time.Sleep(100 * time.Millisecond)
	cancel() // Stop monitoring
	_ = cmd.Process.Kill()

	select {
	case res := <-handle.ResultCh:
		if !res.AutoMarked {
			t.Error("expected auto-marked to be true at 95% progress")
		}
		marks = client.GetMarks()
		found = false
		for _, m := range marks {
			if m == "2" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected item 2 to be marked watched on exit")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for scrobble result")
	}
}

// mockMPV is a fake mpv IPC server that answers property requests and pushes
// property-change events, letting us exercise the event-driven timeline path.
type mockMPV struct {
	pos      float64
	paused   bool
	mu       sync.Mutex
	wmu      sync.Mutex
	socket   string
	listener net.Listener
}

func (m *mockMPV) setPos(p float64) { m.mu.Lock(); m.pos = p; m.mu.Unlock() }
func (m *mockMPV) setPaused(p bool) { m.mu.Lock(); m.paused = p; m.mu.Unlock() }

func startMockMPV(t *testing.T) *mockMPV {
	t.Helper()
	dir, err := os.MkdirTemp("", "cue-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "mpv.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockMPV{socket: sock, listener: l}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go m.handle(c)
		}
	}()
	return m
}

func (m *mockMPV) writeEvent(c net.Conn, event string, data interface{}) {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	b, _ := json.Marshal(map[string]interface{}{"event": event, "data": data})
	_, _ = c.Write(append(b, '\n'))
}

func (m *mockMPV) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	done := make(chan struct{})
	defer close(done)
	dec := json.NewDecoder(c)

	// Simulate mpv pushing property-change events while "playing".
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				m.mu.Lock()
				pos := m.pos
				paused := m.paused
				m.mu.Unlock()
				m.writeEvent(c, "property-change", map[string]interface{}{"name": "time-pos", "data": pos, "id": 1})
				m.writeEvent(c, "property-change", map[string]interface{}{"name": "pause", "data": paused, "id": 2})
			}
		}
	}()

	for {
		var req struct {
			Command   []interface{} `json:"command"`
			RequestID int64         `json:"request_id"`
		}
		if err := dec.Decode(&req); err != nil {
			return
		}
		if len(req.Command) == 0 {
			continue
		}
		var data interface{}
		if req.Command[0] == "get_property" {
			prop := req.Command[1].(string)
			m.mu.Lock()
			switch prop {
			case "time-pos":
				data = m.pos
			case "playlist-pos":
				data = float64(0)
			case "pause":
				data = m.paused
			}
			m.mu.Unlock()
		}
		m.wmu.Lock()
		resp, _ := json.Marshal(map[string]interface{}{"data": data, "error": "success", "request_id": req.RequestID})
		_, _ = c.Write(append(resp, '\n'))
		m.wmu.Unlock()
	}
}

// TestScrobblerTimelineResumeAndSeek verifies two reported issues:
//  1. Resuming reports the correct starting offset (not 0).
//  2. Seeking and pausing are reflected immediately via mpv events.
func TestScrobblerTimelineResumeAndSeek(t *testing.T) {
	mock := startMockMPV(t)
	mock.setPos(0) // GetTimePos reports 0, so the resume offset must be used

	client := &mockPlaybackClient{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewScrobbler(client, logger)
	s.interval = 50 * time.Millisecond

	items := []domain.MediaItem{
		{ID: "1", Title: "Ep 1", Duration: 100 * time.Second},
	}

	cmd := exec.Command("sleep", "2")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle := s.Monitor(ctx, cmd, mock.socket, 0, 30000, items...)
	defer func() { _ = cmd.Process.Kill() }()

	time.Sleep(200 * time.Millisecond)

	// 1. Resume offset must be reported as the starting position, not 0.
	sawOffset := false
	for _, tr := range client.GetTimelines() {
		if tr.RatingKey == "1" && tr.State == "playing" && tr.TimeMs >= 30000 {
			sawOffset = true
		}
	}
	if !sawOffset {
		t.Errorf("expected initial timeline report at resume offset (>=30000ms), got: %+v", client.GetTimelines())
	}

	// 2. Seeking must update the timer immediately via mpv events.
	mock.setPos(50.0)
	time.Sleep(200 * time.Millisecond)
	sawSeek := false
	for _, tr := range client.GetTimelines() {
		if tr.RatingKey == "1" && tr.State == "playing" && tr.TimeMs >= 50000 {
			sawSeek = true
		}
	}
	if !sawSeek {
		t.Errorf("expected timeline update after seek to ~50000ms, got: %+v", client.GetTimelines())
	}

	// 3. Pausing must flip the reported session state to "paused".
	mock.setPaused(true)
	time.Sleep(200 * time.Millisecond)
	sawPaused := false
	for _, tr := range client.GetTimelines() {
		if tr.RatingKey == "1" && tr.State == "paused" {
			sawPaused = true
		}
	}
	if !sawPaused {
		t.Errorf("expected timeline state 'paused' after pause, got: %+v", client.GetTimelines())
	}

	cancel()
	<-handle.ResultCh

	// The terminal state must be sent after all queued playback updates so a
	// delayed playing event cannot recreate the session after playback exits.
	timelines := client.GetTimelines()
	if len(timelines) == 0 || timelines[len(timelines)-1].State != "stopped" {
		t.Errorf("expected final timeline state to be stopped, got: %+v", timelines)
	}
}
