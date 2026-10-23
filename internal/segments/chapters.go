package segments

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

type chapterDisplay struct {
	Name     string `xml:"ChapterString"`
	Language string `xml:"ChapterLanguage"`
}
type chapterAtom struct {
	Start   string         `xml:"ChapterTimeStart"`
	Display chapterDisplay `xml:"ChapterDisplay"`
}
type chaptersXML struct {
	XMLName xml.Name      `xml:"Chapters"`
	Atoms   []chapterAtom `xml:"EditionEntry>ChapterAtom"`
}

func chapterTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d000000", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}

// ChapterXML exports runtime-equivalent boundaries for an explicit file writer.
func ChapterXML(input []domain.SkipSegment, durationMs int64) ([]byte, error) {
	valid := domain.ValidSkipSegments(input, durationMs)
	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid skip segments")
	}
	atoms := []chapterAtom{{Start: chapterTime(0), Display: chapterDisplay{"Episode", "eng"}}}
	add := func(ms int64, title string) {
		a := chapterAtom{Start: chapterTime(ms), Display: chapterDisplay{title, "eng"}}
		if atoms[len(atoms)-1].Start == a.Start {
			atoms[len(atoms)-1] = a
		} else {
			atoms = append(atoms, a)
		}
	}
	for _, s := range valid {
		add(s.StartMs, s.Kind)
		if s.EndMs < durationMs {
			add(s.EndMs, "Episode")
		}
	}
	b, err := xml.MarshalIndent(chaptersXML{Atoms: atoms}, "", "  ")
	return append([]byte(xml.Header), b...), err
}

type fileProbe struct {
	Format struct {
		Duration string `json:"duration"`
		Name     string `json:"format_name"`
	} `json:"format"`
	Streams  []map[string]interface{} `json:"streams"`
	Chapters []struct {
		Start string            `json:"start_time"`
		Tags  map[string]string `json:"tags"`
	} `json:"chapters"`
}

func probeFile(ctx context.Context, path string) (fileProbe, error) {
	var probe fileProbe
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration,format_name:stream=index,codec_name,codec_type:stream_tags:chapter=start_time:chapter_tags=title", "-of", "json", path)
	output := &limitedBuffer{limit: 2 * 1024 * 1024}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return probe, fmt.Errorf("could not probe original media (ffprobe required)")
	}
	if err := json.Unmarshal(output.Bytes(), &probe); err != nil {
		return probe, err
	}
	return probe, nil
}

// EmbedMKV stages a full copy and preserves the original with a hard-link backup.
// It refuses existing chapters and changed originals; no in-place editing occurs.
func EmbedMKV(ctx context.Context, path string, segments []domain.SkipSegment, durationMs int64) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(path), ".mkv") {
		return fmt.Errorf("chapter embedding currently supports MKV only")
	}
	if _, err := exec.LookPath("mkvpropedit"); err != nil {
		return fmt.Errorf("mkvpropedit is required for chapter embedding")
	}
	lock, err := os.OpenFile(path+".cue-chapters.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("could not lock original media")
	}
	_ = lock.Close()
	defer func() { _ = os.Remove(path + ".cue-chapters.lock") }()
	original, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !original.Mode().IsRegular() {
		return fmt.Errorf("original must be a regular file, not a symlink")
	}
	before, err := probeFile(ctx, path)
	if err != nil {
		return err
	}
	if len(before.Chapters) > 0 {
		return fmt.Errorf("original already contains chapters; left unchanged")
	}
	duration, err := strconv.ParseFloat(before.Format.Duration, 64)
	if err != nil || math.Abs(duration*1000-float64(durationMs)) > 1000 {
		return fmt.Errorf("original duration does not match selected media source")
	}
	if !strings.Contains(before.Format.Name, "matroska") {
		return fmt.Errorf("original is not Matroska")
	}
	xmlData, err := ChapterXML(segments, durationMs)
	if err != nil {
		return err
	}
	metadata, err := os.CreateTemp(filepath.Dir(path), ".cue-chapters-*.xml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(metadata.Name()) }()
	if _, err := metadata.Write(xmlData); err != nil {
		_ = metadata.Close()
		return err
	}
	if err := metadata.Close(); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(path), ".cue-media-*.mkv")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	source, err := os.Open(path)
	if err != nil {
		_ = staged.Close()
		return err
	}
	_, err = io.Copy(staged, source)
	_ = source.Close()
	closeErr := staged.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "mkvpropedit", staged.Name(), "--chapters", metadata.Name())
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chapter embedding failed; original unchanged")
	}
	after, err := probeFile(ctx, staged.Name())
	if err != nil {
		return err
	}
	if len(after.Chapters) == 0 || !reflect.DeepEqual(before.Streams, after.Streams) {
		return fmt.Errorf("chapter/stream verification failed; original unchanged")
	}
	expected := chaptersXML{}
	if err := xml.Unmarshal(xmlData, &expected); err != nil {
		return err
	}
	if len(after.Chapters) != len(expected.Atoms) {
		return fmt.Errorf("chapter count verification failed")
	}
	for i, c := range after.Chapters {
		seconds, err := strconv.ParseFloat(c.Start, 64)
		if err != nil || c.Tags["title"] != expected.Atoms[i].Display.Name || chapterTime(int64(math.Round(seconds*1000))) != expected.Atoms[i].Start {
			return fmt.Errorf("chapter boundary verification failed")
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(original, current) || current.Size() != original.Size() || !current.ModTime().Equal(original.ModTime()) {
		return fmt.Errorf("original changed during chapter generation")
	}
	if err := os.Chmod(staged.Name(), original.Mode().Perm()); err != nil {
		return err
	}
	backup := path + ".cue-chapters.bak"
	if err := os.Link(path, backup); err != nil {
		return fmt.Errorf("could not preserve original backup (existing backups are never overwritten)")
	}
	if err := os.Rename(staged.Name(), path); err != nil {
		_ = os.Remove(backup)
		return err
	}
	return nil
}
