package player

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/segments"
)

func TestMPVMovieSubtitlePreferencePersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	script, err := prepareScopedItemPlaybackSettings("server", "user", "movie", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(script) })
	extra := []string{"--script=" + script}
	for i := 1; i <= 3; i++ {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("sub%d.srt", i))
		if err := os.WriteFile(path, []byte("1\n00:00:00,000 --> 00:00:10,000\nSubtitle\n"), 0600); err != nil {
			t.Fatal(err)
		}
		extra = append(extra, "--sub-file="+path)
	}
	connection := startTestMPV(t, config.DefaultSkipConfig(), []domain.PlayableMedia{{DurationMs: 12000}}, extra)
	settings := filepath.Join(filepath.Dir(script), "settings.json")
	deadline := time.Now().Add(2 * time.Second)
	var value struct {
		SID int `json:"sid"`
	}
	for time.Now().Before(deadline) {
		if _, err := connection.request([]interface{}{"set_property", "sid", 3}); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(settings)
		if json.Unmarshal(data, &value) == nil && value.SID == 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if value.SID != 3 {
		t.Fatal("movie subtitle selection was not saved")
	}
	if err := segments.DefaultCache().PruneShows("server", "user", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings); err != nil {
		t.Fatalf("show pruning removed movie preferences: %v", err)
	}
	second := startTestMPV(t, config.DefaultSkipConfig(), []domain.PlayableMedia{{DurationMs: 12000}}, extra)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sid, _ := second.GetProperty("sid")
		if sid == float64(3) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("movie subtitle selection was not restored on the next launch")
}
