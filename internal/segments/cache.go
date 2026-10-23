package segments

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

const Version = "chromaprint-v1"

// Identity deliberately excludes playback URLs and authentication credentials.
type Identity struct {
	Server, User, Show, Item, Source, Revision string
	DurationMs                                 int64
}
type Cached struct {
	SeasonID           string
	Membership         string
	Identity           Identity
	Version            string
	WindowSeconds      int
	IntroWindowSeconds int
	AudioTrack         int
	Head, Tail         []uint32
	Segments           []domain.SkipSegment
}
type Cache struct{ Root string }

func DefaultCache() Cache {
	root, err := os.UserConfigDir()
	if err != nil {
		return Cache{}
	}
	return Cache{filepath.Join(root, "cue", "playback-settings")}
}
func (c Cache) path(id Identity) string {
	b, _ := json.Marshal(id)
	return filepath.Join(c.ShowDir(id.Server, id.User, id.Show), "analysis", fmt.Sprintf("%x.json", sha256.Sum256(b)))
}
func (c Cache) Load(id Identity) (Cached, bool) {
	var value Cached
	if c.Root == "" {
		return value, false
	}
	b, err := os.ReadFile(c.path(id))
	if err != nil || json.Unmarshal(b, &value) != nil || value.Identity != id || value.Version != Version {
		return Cached{}, false
	}
	return value, true
}
func (c Cache) Save(value Cached) error {
	if c.Root == "" {
		return fmt.Errorf("segment cache unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(c.path(value.Identity)), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(c.path(value.Identity)), ".segments-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.path(value.Identity))
}

func hashID(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
func (c Cache) ServerDir(server, user string) string {
	return filepath.Join(c.Root, hashID(server+"|"+user))
}
func (c Cache) ShowDir(server, user, show string) string {
	return filepath.Join(c.ServerDir(server, user), hashID(show))
}

// PruneShows is called only after a complete successful inventory. Scope is
// restricted to this server/account; another profile's settings remain intact.
func (c Cache) PruneShows(server, user string, shows []string) error {
	if c.Root == "" {
		return nil
	}
	keep := map[string]bool{}
	for _, show := range shows {
		keep[hashID(show)] = true
	}
	dirs, err := os.ReadDir(c.ServerDir(server, user))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range dirs {
		if entry.IsDir() && len(entry.Name()) == 64 && !keep[entry.Name()] {
			if err := os.RemoveAll(filepath.Join(c.ServerDir(server, user), entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// PruneEpisodes runs only after every season's episode inventory succeeded.
func (c Cache) PruneEpisodes(server, user, show string, items []string) error {
	directory := filepath.Join(c.ShowDir(server, user, show), "analysis")
	keep := map[string]bool{}
	for _, item := range items {
		keep[item] = true
	}
	files, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, file.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value Cached
		if json.Unmarshal(b, &value) != nil {
			continue
		}
		if !keep[value.Identity.Item] {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c Cache) SegmentFile(id Identity) string {
	if c.Root == "" {
		return ""
	}
	return c.path(id)
}
