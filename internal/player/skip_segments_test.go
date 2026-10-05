package player

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
)

func silentWAV(t *testing.T) string {
	t.Helper()
	data := make([]byte, 44+12*8000*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	path := filepath.Join(t.TempDir(), "episode.wav")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func startTestMPV(t *testing.T, options config.SkipConfig, media []domain.PlayableMedia, extra []string) *mpvConn {
	t.Helper()
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv unavailable")
	}
	script, err := prepareSkipScript(options, media, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(script) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	socket := newMPVSocketPath()
	args := []string{"--no-config", "--vo=null", "--ao=null", "--pause", "--idle=yes", "--input-ipc-server=" + socket, "--script=" + script}
	args = append(args, extra...)
	for range media {
		args = append(args, silentWAV(t))
	}
	cmd := exec.CommandContext(ctx, "mpv", args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait(); os.Remove(socket) })
	var connection *mpvConn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		connection, err = dialMPV(socket)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return connection
}
func TestMPVSkipPromptSeekUndoAndChapters(t *testing.T) {
	connection := startTestMPV(t, config.DefaultSkipConfig(), []domain.PlayableMedia{{DurationMs: 12000, Segments: []domain.SkipSegment{{Kind: "intro", StartMs: 1000, EndMs: 5000, Origin: "test"}}}}, nil)

	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("mpv condition timed out")
	}
	wait(func() bool {
		v, _ := connection.GetProperty("chapter-list")
		a, ok := v.([]interface{})
		return ok && len(a) == 3
	})
	if _, err := connection.request([]interface{}{"seek", 2, "absolute", "exact"}); err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		v, _ := connection.GetProperty("input-bindings")
		a, _ := v.([]interface{})
		for _, v := range a {
			b, _ := v.(map[string]interface{})
			if b["key"] == "Ctrl+x" {
				cmd, _ := b["cmd"].(string)
				if len(cmd) > 0 && strings.Contains(cmd, "cue-skip") {
					return true
				}
			}
		}
		return false
	})
	if _, err := connection.request([]interface{}{"keypress", "Ctrl+x"}); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { v, _ := connection.GetTimePos(); return v >= 4.9 && v < 5.2 })
	if _, err := connection.request([]interface{}{"keypress", "Alt+x"}); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { v, _ := connection.GetTimePos(); return v >= 1.9 && v < 2.2 })
}

func mpvWait(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("mpv condition timed out")
}
func TestMPVRespectsUserBindingAndExistingChapters(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.conf")
	if err := os.WriteFile(input, []byte("Ctrl+x set volume 37\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "chapters.lua")
	if err := os.WriteFile(fixture, []byte("mp.register_event('file-loaded',function() mp.set_property_native('chapter-list',{{time=0,title='Original'},{time=6,title='Story'}}) end)"), 0600); err != nil {
		t.Fatal(err)
	}
	connection := startTestMPV(t, config.DefaultSkipConfig(), []domain.PlayableMedia{{DurationMs: 12000, Segments: []domain.SkipSegment{{Kind: "intro", StartMs: 1000, EndMs: 5000}}}}, []string{"--input-conf=" + input, "--script=" + fixture})
	mpvWait(t, func() bool {
		v, _ := connection.GetProperty("chapter-list")
		a, _ := v.([]interface{})
		return len(a) == 2
	})
	connection.request([]interface{}{"seek", 2, "absolute", "exact"})
	time.Sleep(100 * time.Millisecond)
	if _, err := connection.request([]interface{}{"keypress", "Ctrl+x"}); err != nil {
		t.Fatal(err)
	}
	mpvWait(t, func() bool { v, _ := connection.GetProperty("volume"); return v == float64(37) })
	pos, _ := connection.GetTimePos()
	if pos > 2.2 {
		t.Fatal("overrode user binding")
	}
	chapters, _ := connection.GetProperty("chapter-list")
	a := chapters.([]interface{})
	if a[0].(map[string]interface{})["title"] != "Original" {
		t.Fatal("replaced original chapters")
	}
}
func TestMPVLateAnalysisAndPlaylistMapping(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "analysis.json")
	connection := startTestMPV(t, config.DefaultSkipConfig(), []domain.PlayableMedia{{DurationMs: 12000}, {DurationMs: 12000, SegmentFile: cache}}, []string{"--playlist-start=1"})
	mpvWait(t, func() bool { v, _ := connection.GetProperty("playlist-pos"); return v == float64(1) })
	if err := os.WriteFile(cache, []byte(`{"Version":"chromaprint-v1","Segments":[{"kind":"intro","start_ms":1000,"end_ms":7000,"manual_only":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	mpvWait(t, func() bool {
		v, _ := connection.GetProperty("chapter-list")
		a, _ := v.([]interface{})
		return len(a) == 3
	})
	connection.request([]interface{}{"seek", 2, "absolute", "exact"})
	time.Sleep(100 * time.Millisecond)
	if _, err := connection.request([]interface{}{"keypress", "Ctrl+x"}); err != nil {
		t.Fatal(err)
	}
	mpvWait(t, func() bool { pos, _ := connection.GetTimePos(); return pos > 6.9 && pos < 7.2 })
	connection.request([]interface{}{"playlist-prev", "force"})
	mpvWait(t, func() bool { v, _ := connection.GetProperty("playlist-pos"); return v == float64(0) })
	mpvWait(t, func() bool {
		v, _ := connection.GetProperty("chapter-list")
		a, _ := v.([]interface{})
		return len(a) == 0
	})
}

func TestMPVDetectedSegmentsFollowConfiguredMode(t *testing.T) {
	for _, kind := range []string{"intro", "outro"} {
		for _, mode := range []string{"auto", "manual", "off"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				options := config.DefaultSkipConfig()
				options.Intro, options.Outro = mode, mode
				// Existing detection caches include this legacy flag. Auto must
				// also work with those without another analysis pass.
				media := []domain.PlayableMedia{{DurationMs: 12000, Segments: []domain.SkipSegment{{Kind: kind, StartMs: 1000, EndMs: 5000, Origin: "chromaprint-v1", ManualOnly: true}}}}
				connection := startTestMPV(t, options, media, nil)
				mpvWait(t, func() bool {
					chapters, _ := connection.GetProperty("chapter-list")
					list, _ := chapters.([]interface{})
					return len(list) == 3
				})
				if _, err := connection.request([]interface{}{"seek", 2, "absolute", "exact"}); err != nil {
					t.Fatal(err)
				}
				if mode == "auto" {
					mpvWait(t, func() bool { pos, _ := connection.GetTimePos(); return pos >= 4.9 && pos < 5.2 })
					if _, err := connection.request([]interface{}{"keypress", "Alt+x"}); err != nil {
						t.Fatal(err)
					}
				}
				mpvWait(t, func() bool { pos, _ := connection.GetTimePos(); return pos >= 1.9 && pos < 2.2 })
				// Undo must not immediately trigger another automatic skip.
				time.Sleep(150 * time.Millisecond)
				pos, err := connection.GetTimePos()
				if err != nil || pos < 1.9 || pos > 2.2 {
					t.Fatalf("mode=%s final position=%v err=%v", mode, pos, err)
				}
			})
		}
	}
}

func TestMPVAutoSkipsLateCachedDetection(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "analysis.json")
	options := config.DefaultSkipConfig()
	options.Intro = "auto"
	connection := startTestMPV(t, options, []domain.PlayableMedia{{DurationMs: 12000, SegmentFile: cache}}, nil)
	if _, err := connection.request([]interface{}{"seek", 2, "absolute", "exact"}); err != nil {
		t.Fatal(err)
	}
	mpvWait(t, func() bool { pos, _ := connection.GetTimePos(); return pos >= 1.9 && pos < 2.2 })
	if err := os.WriteFile(cache, []byte(`{"Version":"chromaprint-v1","Segments":[{"kind":"intro","start_ms":1000,"end_ms":7000,"manual_only":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	mpvWait(t, func() bool { pos, _ := connection.GetTimePos(); return pos >= 6.9 && pos < 7.2 })
}
