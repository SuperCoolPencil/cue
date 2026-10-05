package segments

import (
	"context"
	"math/rand"
	"testing"
)

func randomFingerprint(seed int64, n int) []uint32 {
	r := rand.New(rand.NewSource(seed))
	fp := make([]uint32, n)
	for i := range fp {
		fp[i] = r.Uint32()
	}
	return fp
}
func TestDetectShiftedRecurringSequences(t *testing.T) {
	theme := randomFingerprint(5, 500)
	var episodes [][]uint32
	for i := 0; i < 3; i++ {
		fp := randomFingerprint(int64(i+20), 1000)
		copy(fp[50+i*80:], theme)
		episodes = append(episodes, fp)
	}
	got, err := Detect(context.Background(), episodes, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for i, segments := range got {
		if len(segments) != 1 || !segments[0].ManualOnly {
			t.Fatalf("episode %d: %+v", i, segments)
		}
		want := int64((float64(50+i*80)*FrameSeconds - boundaryMargin) * 1000)
		if segments[0].StartMs != want {
			t.Fatalf("episode %d offset: %+v want %d", i, segments, want)
		}
	}
}
func TestDetectAbstainsForSilenceAndInsufficientSupport(t *testing.T) {
	for _, episodes := range [][][]uint32{{make([]uint32, 600), make([]uint32, 600), make([]uint32, 600)}, {randomFingerprint(1, 600), randomFingerprint(1, 600), randomFingerprint(2, 600)}} {
		got, err := Detect(context.Background(), episodes, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range got {
			if len(s) != 0 {
				t.Fatalf("false match: %+v", s)
			}
		}
	}
}
func TestDetectCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Detect(ctx, [][]uint32{randomFingerprint(1, 600)}, []int64{200000}, []int64{0}, "intro")
	if err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestCacheRevision(t *testing.T) {
	c := Cache{Root: t.TempDir()}
	id := Identity{Server: "server", Item: "episode", Source: "version", Revision: "old", DurationMs: 100000}
	entry := Cached{Identity: id, Version: Version, Head: []uint32{1, 2}}
	if err := c.Save(entry); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(id); !ok {
		t.Fatal("cache miss")
	}
	id.Revision = "new"
	if _, ok := c.Load(id); ok {
		t.Fatal("stale revision reused")
	}
}

func TestDetectToleratesBriefFingerprintDifferences(t *testing.T) {
	theme := randomFingerprint(5, 500)
	episodes := [][]uint32{append([]uint32(nil), theme...), append([]uint32(nil), theme...), append([]uint32(nil), theme...)}
	// Interrupt every twelve seconds: no uninterrupted passage reaches 25s.
	for start := 90; start < 450; start += 90 {
		for n := start; n < start+5; n++ {
			episodes[0][n] ^= 0xffffffff
		}
	}
	got, err := Detect(context.Background(), episodes, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range got {
		if len(s) != 1 || s[0].EndMs < 60000 {
			t.Fatalf("episode %d: %+v", i, s)
		}
	}
}

func TestDetectRejectsMostlyMatchingShortFragments(t *testing.T) {
	theme := randomFingerprint(8, 600)
	altered := append([]uint32(nil), theme...)
	for i := 0; i < len(altered); i++ {
		if i%10 < 2 {
			altered[i] ^= 0xffffffff
		}
	}
	got, err := Detect(context.Background(), [][]uint32{altered, theme, theme}, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got {
		if len(s) > 0 {
			t.Fatalf("accepted 20%% mismatch: %+v", s)
		}
	}
}

func TestConsensusCountsDistinctPeers(t *testing.T) {
	cases := []struct {
		name    string
		support [][]match
		want    []match
	}{
		{"short outlier", [][]match{{{100, 600}}, {{100, 600}}, {{140, 550}}}, []match{{100, 600}}},
		{"single long estimate", [][]match{{{100, 600}}, {{140, 550}}}, []match{{140, 550}}},
		{"duplicates", [][]match{{{100, 600}, {100, 600}, {120, 580}}}, nil},
		{"separate regions", [][]match{{{0, 300}, {500, 800}}, {{0, 300}, {500, 800}}}, []match{{0, 300}, {500, 800}}},
		{"short overlap", [][]match{{{0, 300}}, {{200, 500}}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := consensusMatches(tc.support, 202)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}
func TestDetectShortVariantDoesNotTrimFullIntros(t *testing.T) {
	theme := randomFingerprint(88, 500)
	var episodes [][]uint32
	for i := 0; i < 4; i++ {
		fp := randomFingerprint(int64(100+i), 1000)
		if i == 3 {
			copy(fp[140:550], theme[40:450])
		} else {
			copy(fp[100:600], theme)
		}
		episodes = append(episodes, fp)
	}
	got, err := Detect(context.Background(), episodes, []int64{200000, 200000, 200000, 200000}, []int64{0, 0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range got {
		start, end := 100, 600
		if i == 3 {
			start, end = 140, 550
		}
		wantStart := int64((float64(start)*FrameSeconds - boundaryMargin) * 1000)
		wantEnd := int64(float64(end) * FrameSeconds * 1000)
		if len(s) != 1 || s[0].StartMs != wantStart || s[0].EndMs != wantEnd {
			t.Fatalf("episode %d: %v, want %d-%d", i, s, wantStart, wantEnd)
		}
	}
}

func TestDetectLateOpeningWithSustainedAudioDifferences(t *testing.T) {
	// Shogun E2: a late, 89-second opening with mix differences around
	// 7:15 and throughout 7:50-8:10. These used to truncate it at 7:53.
	onset, finish := 420.0, 509.0
	start := int((onset + boundaryMargin) / FrameSeconds)
	end := int(finish / FrameSeconds)
	theme := randomFingerprint(701, end-start)
	var episodes [][]uint32
	for i := 0; i < 3; i++ {
		fp := randomFingerprint(int64(710+i), 4825)
		at := start - i*700
		copy(fp[at:], theme)
		if i == 0 {
			for n := 0; n < len(theme); n++ {
				if (n >= 90 && n < 120) || (n >= 370 && n < 540) {
					fp[at+n] ^= 0x3f // Six changed bits, still the same passage.
				}
			}
		}
		episodes = append(episodes, fp)
	}
	got, err := Detect(context.Background(), episodes, []int64{3600000, 3600000, 3600000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0]) != 1 || got[0][0].StartMs < 419000 || got[0][0].StartMs > 421000 || got[0][0].EndMs < 508000 || got[0][0].EndMs > 510000 {
		t.Fatalf("late opening was truncated: %v", got[0])
	}
}

func TestDetectRejectsDistantFingerprints(t *testing.T) {
	theme := randomFingerprint(901, 700)
	altered := append([]uint32(nil), theme...)
	for i := range altered {
		altered[i] ^= 0x7f // Seven changed bits exceed the tolerance.
	}
	got, err := Detect(context.Background(), [][]uint32{altered, theme, theme}, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for _, segments := range got {
		if len(segments) != 0 {
			t.Fatalf("accepted distant audio: %v", segments)
		}
	}
}

func TestDetectOpeningAtZero(t *testing.T) {
	theme := randomFingerprint(902, 700)
	got, err := Detect(context.Background(), [][]uint32{theme, theme, theme}, []int64{200000, 200000, 200000}, []int64{0, 0, 0}, "intro")
	if err != nil {
		t.Fatal(err)
	}
	for _, segments := range got {
		if len(segments) != 1 || segments[0].StartMs != 0 {
			t.Fatalf("lost opening at start of episode: %v", segments)
		}
	}
}
