package segments

import (
	"context"
	"math/bits"
	"sort"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

// Chromaprint algorithm 1: 1365 samples per frame at 11025 Hz. The
// filtering/classifier delay spans ~2.6s. Leading fingerprints straddle the
// transition into an opening, so recover its onset from the first stable match.
// The player's configured mode decides whether matches skip automatically.
const FrameSeconds = 1365.0 / 11025.0
const boundaryMargin = 3.0
const maxFingerprintDistance = 6

type match struct{ start, end int }

func diverse(fp []uint32) bool {
	values := map[uint32]bool{}
	for _, v := range fp {
		values[v] = true
	}
	return len(values) >= 16
}

// pairMatches scans offset diagonals, allowing short fingerprint interruptions.
// Allow small bit differences from audio encoding/mixing, but require at least
// 90% of each candidate to match; silence fails the diversity check.
func pairMatches(ctx context.Context, a, b []uint32, minFrames int) ([]match, error) {
	var matches []match
	for offset := -len(b) + minFrames; offset <= len(a)-minFrames; offset++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		i, j := offset, 0
		if i < 0 {
			j = -i
			i = 0
		}
		start, lastGood, good := -1, -1, 0
		flush := func(end int) {
			end = lastGood + 1
			if start >= 0 && end-start >= minFrames && good*10 >= (end-start)*9 && diverse(a[start:end]) {
				matches = append(matches, match{start, end})
			}
			start, lastGood, good = -1, -1, 0
		}
		for i < len(a) && j < len(b) {
			if bits.OnesCount32(a[i]^b[j]) <= maxFingerprintDistance {
				if start < 0 {
					start = i
				}
				lastGood = i
				good++
			} else if start >= 0 && i-lastGood > 8 {
				flush(i)
			}
			i++
			j++
		}
		flush(i)
	}
	return matches, nil
}

// consensusMatches retains regions supported by at least two distinct peers.
// Merge each peer's estimates first so repeated matches cannot inflate support.
// A shorter peer match cannot truncate edges confirmed by two other peers.
func consensusMatches(support [][]match, minFrames int) []match {
	type event struct{ frame, delta int }
	var events []event
	for _, estimates := range support {
		estimates = append([]match(nil), estimates...)
		sort.Slice(estimates, func(i, j int) bool { return estimates[i].start < estimates[j].start })
		var merged []match
		for _, m := range estimates {
			if m.end <= m.start {
				continue
			}
			if len(merged) > 0 && m.start <= merged[len(merged)-1].end {
				merged[len(merged)-1].end = max(merged[len(merged)-1].end, m.end)
			} else {
				merged = append(merged, m)
			}
		}
		for _, m := range merged {
			events = append(events, event{m.start, 1}, event{m.end, -1})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].frame < events[j].frame })
	var result []match
	count, start := 0, 0
	for i := 0; i < len(events); {
		frame, delta := events[i].frame, 0
		for i < len(events) && events[i].frame == frame {
			delta += events[i].delta
			i++
		}
		next := count + delta
		if count < 2 && next >= 2 {
			start = frame
		}
		if count >= 2 && next < 2 && frame-start >= minFrames {
			result = append(result, match{start, frame})
		}
		count = next
	}
	return result
}

// Detect requires the same bounded passage to match two other episodes.
// Each episode is aligned independently, allowing shifted cold opens and
// different opening clusters within a season. Conflicting candidates abstain.
func Detect(ctx context.Context, fingerprints [][]uint32, durations []int64, offsets []int64, kind string) ([][]domain.SkipSegment, error) {
	out := make([][]domain.SkipSegment, len(fingerprints))
	minFrames := 202
	for i, a := range fingerprints {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i >= len(durations) || i >= len(offsets) {
			continue
		}
		support := make([][]match, len(fingerprints))
		for j, b := range fingerprints {
			if i == j {
				continue
			}
			m, err := pairMatches(ctx, a, b, minFrames)
			if err != nil {
				return nil, err
			}
			support[j] = m
		}
		unique := consensusMatches(support, minFrames)
		for _, m := range unique {
			startSeconds := float64(m.start)*FrameSeconds + boundaryMargin
			if kind == "intro" {
				startSeconds = max(0, float64(m.start)*FrameSeconds-boundaryMargin)
			}
			start := offsets[i] + int64(startSeconds*1000)
			end := offsets[i] + int64(float64(m.end)*FrameSeconds*1000)
			if end-start < 20000 {
				continue
			}
			out[i] = append(out[i], domain.SkipSegment{Kind: kind, StartMs: start, EndMs: end, Origin: Version})
		}
		out[i] = domain.ValidSkipSegments(out[i], durations[i])
	}
	return out, nil
}
