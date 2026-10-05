package domain

import "sort"

// SkipSegment describes a bounded interval on the original media timeline.
type SkipSegment struct {
	Kind       string `json:"kind"`
	StartMs    int64  `json:"start_ms"`
	EndMs      int64  `json:"end_ms"`
	Origin     string `json:"origin"`
	ManualOnly bool   `json:"manual_only,omitempty"` // Legacy cache field; playback follows the configured mode.
}

// ValidSkipSegments discards invalid and overlapping intervals conservatively.
func ValidSkipSegments(input []SkipSegment, durationMs int64) []SkipSegment {
	sorted := append([]SkipSegment(nil), input...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	var out []SkipSegment
	for i, s := range sorted {
		if (s.Kind != "intro" && s.Kind != "outro") || s.StartMs < 0 || s.EndMs <= s.StartMs || durationMs <= 0 || s.EndMs > durationMs {
			continue
		}
		conflict := false
		for j, t := range sorted {
			if i != j && t.StartMs < s.EndMs && t.EndMs > s.StartMs {
				conflict = true
				break
			}
		}
		if !conflict {
			out = append(out, s)
		}
	}
	return out
}
