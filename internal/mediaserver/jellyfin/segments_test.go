package jellyfin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolvePlayableSegments(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "old-server"}[unavailable], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/Items/episode/PlaybackInfo" {
					_, _ = w.Write([]byte(`{"MediaSources":[{"Id":"source","Container":"mkv","Size":900,"RunTimeTicks":1200000000}]}`))
					return
				}
				if r.URL.Path != "/MediaSegments/episode" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if unavailable {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(`{"Items":[{"Type":"Intro","StartTicks":10000000,"EndTicks":110000000},{"Type":"Outro","StartTicks":900000000,"EndTicks":1100000000},{"Type":"Recap","StartTicks":0,"EndTicks":10000000}]}`))
			}))
			defer server.Close()
			media, err := NewClient(server.URL, "token", "user", "device", nil).ResolvePlayable(context.Background(), "episode")
			if err != nil {
				t.Fatal(err)
			}
			if media.SourceID != "source" || media.DurationMs != 120000 {
				t.Fatalf("media=%+v", media)
			}
			if unavailable {
				if len(media.Segments) != 0 {
					t.Fatal("unexpected segments")
				}
			} else if len(media.Segments) != 2 || media.Segments[0].StartMs != 1000 || media.Segments[1].EndMs != 110000 {
				t.Fatalf("segments=%+v", media.Segments)
			}
		})
	}
}
