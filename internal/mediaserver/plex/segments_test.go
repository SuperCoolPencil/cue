package plex

import (
	"context"
	"net/http"
	"testing"
)

func TestResolvePlayableMarkers(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeMarkers") != "1" {
			t.Error("markers not requested")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"duration":120000,"updatedAt":10,"Marker":[{"type":"intro","startTimeOffset":1000,"endTimeOffset":11000},{"type":"credits","startTimeOffset":90000,"endTimeOffset":110000}],"Media":[{"Part":[{"id":7,"size":900,"key":"/media"}]}]}]}}`))
	}))
	media, err := client.ResolvePlayable(context.Background(), "episode")
	if err != nil {
		t.Fatal(err)
	}
	if media.SourceID != "7" || media.DurationMs != 120000 || len(media.Segments) != 2 || media.Segments[1].Kind != "outro" || media.Segments[0].EndMs != 11000 {
		t.Fatalf("media=%+v", media)
	}
}
