package mediaserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/mediaserver/plex"
)

// DiscoveredServer represents a single selectable server connection found via
// plex.tv. A single Plex Media Server may expose multiple connections (local
// and remote); each is exposed as its own option, matching plexctl's behavior.
type DiscoveredServer struct {
	Name   string
	ID     string
	Token  string
	URI    string
	Local  bool
	Owned  bool
}

// DiscoverPlexServers returns all non-relay Plex server connections reachable by
// the account configured in cfg. It is only supported for the Plex backend.
func DiscoverPlexServers(ctx context.Context, cfg *config.Config) ([]DiscoveredServer, error) {
	if cfg.Server.Type != config.SourceTypePlex {
		return nil, fmt.Errorf("server discovery is only supported for Plex")
	}
	if cfg.Server.Token == "" {
		return nil, fmt.Errorf("no Plex token configured; run cue to set up your server first")
	}

	resources, err := plex.DiscoverServers(ctx, cfg.Server.Token, cfg.Server.DeviceID)
	if err != nil {
		return nil, err
	}

	out := make([]DiscoveredServer, 0)
	for _, r := range resources {
		if !strings.Contains(r.Provides, "server") {
			continue
		}
		for _, c := range r.Connections {
			if c.Relay || c.URI == "" {
				continue
			}
			out = append(out, DiscoveredServer{
				Name:  r.Name,
				ID:    r.ClientIdentifier,
				Token: r.AccessToken,
				URI:   c.URI,
				Local: c.Local,
				Owned: r.Owned,
			})
		}
	}
	return out, nil
}
