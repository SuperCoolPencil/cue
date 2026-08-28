package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
)

func TestDiscoverServers(t *testing.T) {
	sample := `[
	  {
	    "name": "Living Room",
	    "clientIdentifier": "abc123",
	    "provides": "server",
	    "accessToken": "server-token-1",
	    "owned": true,
	    "connections": [
	      {"protocol": "http", "address": "192.168.1.10", "port": 32400, "uri": "http://192.168.1.10:32400", "local": true, "relay": false},
	      {"protocol": "https", "address": "pytp.example.plex.direct", "port": 32400, "uri": "https://pytp.example.plex.direct:32400", "local": false, "relay": false}
	    ]
	  },
	  {
	    "name": "Friend Server",
	    "clientIdentifier": "def456",
	    "provides": "server",
	    "accessToken": "server-token-2",
	    "owned": false,
	    "connections": [
	      {"protocol": "https", "address": "relay.plex.direct", "port": 32400, "uri": "https://relay.plex.direct:32400", "local": false, "relay": true}
	    ]
	  },
	  {
	    "name": "Phone",
	    "clientIdentifier": "ghi789",
	    "provides": "controller,player",
	    "accessToken": "",
	    "owned": true,
	    "connections": []
	  }
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("X-Plex-Token"); got != "account-token" {
			t.Errorf("expected X-Plex-Token query param, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sample))
	}))
	defer srv.Close()

	// Point the discovery request at our test server.
	orig := plexTVBaseURL
	plexTVBaseURL = srv.URL
	defer func() { plexTVBaseURL = orig }()

	// Silence viper noise from normalizeClientID path (unused) is fine.
	_ = viper.New()

	servers, err := DiscoverServers(context.Background(), "account-token", "cue-test")
	if err != nil {
		t.Fatalf("DiscoverServers returned error: %v", err)
	}

	// Only the two "server" devices should remain; the "controller,player"
	// device must be filtered out even though it is owned.
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Name != "Living Room" || servers[0].AccessToken != "server-token-1" {
		t.Errorf("unexpected first server: %+v", servers[0])
	}
	if servers[1].Name != "Friend Server" {
		t.Errorf("unexpected second server: %+v", servers[1])
	}

	// Empty token must error.
	if _, err := DiscoverServers(context.Background(), "", "cue-test"); err == nil {
		t.Error("expected error for empty token")
	}
}
