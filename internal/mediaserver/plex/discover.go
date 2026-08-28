package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PlexResourceConnection represents a single connection URI advertised by a
// Plex resource (server) on plex.tv.
type PlexResourceConnection struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
	IPv6     bool   `json:"ipv6"`
}

// PlexResource represents a device returned by the plex.tv resources endpoint.
type PlexResource struct {
	Name               string                   `json:"name"`
	ClientIdentifier   string                   `json:"clientIdentifier"`
	Provides           string                   `json:"provides"`
	Product            string                   `json:"product"`
	ProductVersion     string                   `json:"productVersion"`
	Platform           string                   `json:"platform"`
	AccessToken         string                  `json:"accessToken"`
	Owned              bool                     `json:"owned"`
	Home               bool                     `json:"home"`
	Presence           bool                     `json:"presence"`
	HTTPSRequired      bool                     `json:"httpsRequired"`
	PublicAddress      string                   `json:"publicAddress"`
	PublicAddressMatches bool                   `json:"publicAddressMatches"`
	Relay              bool                     `json:"relay"`
	Connections        []PlexResourceConnection `json:"connections"`
}

// DiscoverServers queries plex.tv for the servers reachable by the Plex account
// identified by the given token. It returns only the devices that provide a
// "server" (Plex Media Server), regardless of ownership.
func DiscoverServers(ctx context.Context, token, clientID string) ([]PlexResource, error) {
	if token == "" {
		return nil, fmt.Errorf("plex token is required to discover servers")
	}

	clientID = normalizeClientID(clientID)

	reqURL := fmt.Sprintf("%s/api/v2/resources", plexTVBaseURL)
	q := url.Values{}
	q.Set("includeHttps", "1")
	q.Set("includeIPv6", "1")
	q.Set("includeRelay", "1")
	q.Set("X-Plex-Token", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Client-Identifier", clientID)
	req.Header.Set("X-Plex-Product", "Cue")
	req.Header.Set("X-Plex-Version", "1.0")
	req.Header.Set("User-Agent", userAgent)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("plex token rejected (unauthorized)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discovery failed: status %d", resp.StatusCode)
	}

	var resources []PlexResource
	if err := json.Unmarshal(body, &resources); err != nil {
		return nil, fmt.Errorf("failed to parse discovery response: %w", err)
	}

	servers := make([]PlexResource, 0, len(resources))
	for _, r := range resources {
		if strings.Contains(r.Provides, "server") {
			servers = append(servers, r)
		}
	}
	return servers, nil
}
