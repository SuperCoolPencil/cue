package player

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/segments"
)

//go:embed skip_segments.lua
var skipSegmentsLua string

func prepareSkipScript(options config.SkipConfig, media []domain.PlayableMedia, settingsScript string, mapPath ...func(string) string) (string, error) {
	entries := make([][]domain.SkipSegment, len(media))
	files := make([]string, len(media))
	for i, m := range media {
		entries[i] = domain.ValidSkipSegments(m.Segments, m.DurationMs)
		files[i] = m.SegmentFile
		if files[i] != "" && len(mapPath) > 0 {
			files[i] = mapPath[0](files[i])
		}
	}
	manifest, err := json.Marshal(struct {
		Version string                 `json:"version"`
		Options config.SkipConfig      `json:"options"`
		Entries [][]domain.SkipSegment `json:"entries"`
		Files   []string               `json:"files"`
	}{segments.Version, options, entries, files})
	if err != nil {
		return "", err
	}
	source := ""
	if settingsScript != "" {
		b, err := os.ReadFile(settingsScript)
		if err != nil {
			return "", err
		}
		source = "do\n" + string(b) + "\nend\n"
	}
	source += "do\nlocal manifest_json = " + strconv.Quote(string(manifest)) + "\n" + skipSegmentsLua + "\nend\n"
	root := ""
	if settingsScript != "" {
		root = filepath.Dir(settingsScript)
	}
	f, err := os.CreateTemp(root, "cue-playback-*.lua")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(source)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
