package segments

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

type inventoryClient struct {
	domain.LibraryClient
	domain.PlaybackClient
	fail    bool
	removed bool
}

func (c *inventoryClient) GetLibraries(context.Context) ([]domain.Library, error) {
	return []domain.Library{{ID: "tv", Type: "show"}}, nil
}
func (c *inventoryClient) GetShows(_ context.Context, _ string, offset, limit int) ([]*domain.Show, int, error) {
	if c.fail {
		return nil, 0, errors.New("offline")
	}
	return []*domain.Show{{ID: "kept"}}, 1, nil
}
func (c *inventoryClient) GetSeasons(context.Context, string) ([]*domain.Season, error) {
	return []*domain.Season{{ID: "season"}}, nil
}
func (c *inventoryClient) GetEpisodes(context.Context, string) ([]*domain.MediaItem, error) {
	out := []*domain.MediaItem{{ID: "a", ShowID: "kept"}, {ID: "b", ShowID: "kept"}, {ID: "c", ShowID: "kept"}}
	if c.removed {
		return out[:2], nil
	}
	return out, nil
}
func (c *inventoryClient) ResolvePlayable(_ context.Context, id string) (domain.PlayableMedia, error) {
	return domain.PlayableMedia{URL: id, SourceID: "source", Revision: "1", DurationMs: 200000}, nil
}
func TestStartupAnalysisCacheAndCleanup(t *testing.T) {
	client := &inventoryClient{}
	cache := Cache{Root: t.TempDir()}
	calls := 0
	analyzer := Analyzer{Client: client, Cache: cache, Server: "server", User: "user", Extractor: func(_ context.Context, _ string, _ int64, _ int, _ int) ([]uint32, error) {
		calls++
		return randomFingerprint(5, 500), nil
	}}
	obsolete := cache.ShowDir("server", "user", "removed")
	other := cache.ShowDir("other", "user", "removed")
	for _, dir := range []string{obsolete, other} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := analyzer.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 6 {
		t.Fatalf("extraction calls=%d", calls)
	}
	if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
		t.Fatal("removed show retained")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("other server pruned")
	}
	if err := analyzer.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 6 {
		t.Fatal("unchanged media re-fingerprinted")
	}
	client.removed = true
	if err := analyzer.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := Identity{Server: "server", User: "user", Show: "kept", Item: "c", Source: "source", Revision: "1", DurationMs: 200000}
	if _, ok := cache.Load(id); ok {
		t.Fatal("removed episode retained")
	}
}
func TestStartupFailureDoesNotPrune(t *testing.T) {
	cache := Cache{Root: t.TempDir()}
	obsolete := cache.ShowDir("server", "user", "removed")
	if err := os.MkdirAll(obsolete, 0700); err != nil {
		t.Fatal(err)
	}
	analyzer := Analyzer{Client: &inventoryClient{fail: true}, Cache: cache, Server: "server", User: "user"}
	if err := analyzer.Startup(context.Background()); err == nil {
		t.Fatal("expected inventory failure")
	}
	if _, err := os.Stat(obsolete); err != nil {
		t.Fatal("pruned on incomplete inventory")
	}
}

func TestAnalysisProgressAndPendingSeasonSummary(t *testing.T) {
	cache := Cache{Root: t.TempDir()}
	client := &inventoryClient{}
	episodes, _ := client.GetEpisodes(context.Background(), "season")
	for _, episode := range episodes {
		episode.ParentID = "season"
	}
	calls := 0
	analyzer := Analyzer{Client: client, Cache: cache, Server: "server", User: "user", Extractor: func(context.Context, string, int64, int, int) ([]uint32, error) {
		progress := SeasonAnalysisProgress("server", "user", "kept", "season")
		if !progress.Active || progress.Total != 3 || progress.Completed != calls/2 {
			t.Fatalf("progress during extraction: %+v", progress)
		}
		if calls == 2 {
			summary, err := cache.SeasonIntroSummary("server", "user", "kept", "season")
			if err != nil || summary.Pending != 1 {
				t.Fatalf("pending season summary: %+v, %v", summary, err)
			}
		}
		calls++
		return randomFingerprint(5, 500), nil
	}}
	if _, err := analyzer.analyze(context.Background(), episodes); err != nil {
		t.Fatal(err)
	}
	if progress := SeasonAnalysisProgress("server", "user", "kept", "season"); progress.Active {
		t.Fatalf("finished analysis still active: %+v", progress)
	}
	analyzer.Extractor = func(context.Context, string, int64, int, int) ([]uint32, error) { return nil, errors.New("failed") }
	analyzer.WindowSeconds = 60
	if _, err := analyzer.analyze(context.Background(), episodes); err == nil {
		t.Fatal("expected extraction failure")
	}
	if progress := SeasonAnalysisProgress("server", "user", "kept", "season"); progress.Active {
		t.Fatalf("failed analysis still active: %+v", progress)
	}
}

type longEpisodeClient struct{ inventoryClient }

func (c *longEpisodeClient) ResolvePlayable(_ context.Context, id string) (domain.PlayableMedia, error) {
	return domain.PlayableMedia{URL: id, SourceID: "source", Revision: "1", DurationMs: 2400000}, nil
}
func TestLateIntroAndIndependentWindowCache(t *testing.T) {
	client := &longEpisodeClient{}
	episodes, _ := client.GetEpisodes(context.Background(), "season")
	cache := Cache{Root: t.TempDir()}
	heads, tails := 0, 0
	analyzer := Analyzer{Client: client, Cache: cache, Extractor: func(_ context.Context, url string, offset int64, seconds, track int) ([]uint32, error) {
		seed := int64(url[0])
		fp := randomFingerprint(seed, int(float64(seconds)/FrameSeconds))
		if offset == 0 {
			heads++
			start := int(520 / FrameSeconds)
			if len(fp) >= start+400 {
				copy(fp[start:], randomFingerprint(999, 400))
			}
		} else {
			tails++
		}
		return fp, nil
	}}
	count, err := analyzer.analyze(context.Background(), episodes)
	if err != nil || count != 3 {
		t.Fatalf("late intro detection: count=%d err=%v", count, err)
	}
	if _, err := analyzer.analyze(context.Background(), episodes); err != nil {
		t.Fatal(err)
	}
	if heads != 3 || tails != 3 {
		t.Fatalf("cache not reused: %d/%d", heads, tails)
	}
	analyzer.IntroWindowSeconds = 300
	count, err = analyzer.analyze(context.Background(), episodes)
	if err != nil || count != 0 {
		t.Fatalf("narrow window retained late markers: %d %v", count, err)
	}
	if heads != 6 || tails != 3 {
		t.Fatalf("outros re-extracted: %d/%d", heads, tails)
	}
}
func TestIntroWindowBounds(t *testing.T) {
	for _, tc := range []struct {
		duration  int64
		cap, want int
	}{
		{2400000, 0, 600}, {1200000, 0, 300}, {2400000, 900, 600}, {3600000, 900, 900}, {200000, 0, 50},
	} {
		if got := introWindowSeconds(tc.duration, tc.cap); got != tc.want {
			t.Fatalf("%+v: got %d", tc, got)
		}
	}
}
