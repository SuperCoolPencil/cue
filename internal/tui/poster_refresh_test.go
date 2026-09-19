package tui

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

type posterClientStub struct {
	domain.LibraryClient
	domain.PlaybackClient
	domain.SearchClient
	domain.PlaylistClient
	item     *domain.MediaItem
	attempts int
	urls     []string
}

func (c *posterClientStub) GetMediaItem(context.Context, string) (*domain.MediaItem, error) {
	return c.item, nil
}

func (c *posterClientStub) DeleteMediaItem(context.Context, string) error {
	return nil
}

func (c *posterClientStub) GetNextUp(context.Context, string) (*domain.MediaItem, error) {
	return nil, nil
}

func (c *posterClientStub) GetImage(_ context.Context, url string) ([]byte, error) {
	c.urls = append(c.urls, url)
	c.attempts++
	if c.attempts < posterFetchAttempts {
		return nil, errors.New("temporary image failure")
	}
	return []byte("image"), nil
}

func (c *posterClientStub) GetWebURL(_ context.Context, itemID string) (string, error) {
	return "http://localhost:32400/web/index.html#!/server/mach123/details?key=%2Flibrary%2Fmetadata%2F" + itemID, nil
}

func TestFetchPosterDataRetriesTransientFailures(t *testing.T) {
	client := &posterClientStub{}

	data, err := fetchPosterData(client, "movie-1", "https://media/poster")
	if err != nil {
		t.Fatalf("fetchPosterData() error = %v", err)
	}
	if string(data) != "image" {
		t.Fatalf("fetchPosterData() data = %q", data)
	}
	if client.attempts != posterFetchAttempts {
		t.Fatalf("image fetch attempts = %d, expected %d", client.attempts, posterFetchAttempts)
	}
}

func TestFetchPosterDataUsesMetadataFallback(t *testing.T) {
	client := &posterClientStub{
		item: &domain.MediaItem{ID: "movie-1", ThumbURL: "https://media/fallback-poster"},
	}

	if _, err := fetchPosterData(client, "movie-1", ""); err != nil {
		t.Fatalf("fetchPosterData() error = %v", err)
	}
	if len(client.urls) == 0 || client.urls[0] != "https://media/fallback-poster" {
		t.Fatalf("fallback image URLs = %#v", client.urls)
	}
}

func TestRenderKittyFitsInspectorHeight(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{100, 120, 140, 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}

	_, _, width, height, err := renderKitty(data.Bytes(), "movie-1", 1, 44, 10)
	if err != nil {
		t.Fatalf("renderKitty() error = %v", err)
	}
	if height > 10 {
		t.Fatalf("poster height = %d, expected at most 10 cells", height)
	}
	if width >= 44 {
		t.Fatalf("poster width = %d, expected width reduction to fit height", width)
	}
}

func TestRenderKittyCropsWideArtworkToPosterShape(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 240, 80))
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}

	_, _, width, height, err := renderKitty(data.Bytes(), "show-1", 1, 20, 0)
	if err != nil {
		t.Fatalf("renderKitty() error = %v", err)
	}
	if width != 20 || height < 15 {
		t.Fatalf("wide artwork rendered as %dx%d cells, expected a 20x15-or-taller poster", width, height)
	}
}

func TestRenderASCIIFitsMaxHeight(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{100, 120, 140, 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}

	out, err := renderASCII(data.Bytes(), 40, 5)
	if err != nil {
		t.Fatalf("renderASCII() error = %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) > 5 {
		t.Fatalf("ASCII poster has %d lines, expected at most 5; got:\n%s", len(lines), out)
	}
}

func TestPosterDimensionsRejectExcessivePixelCount(t *testing.T) {
	fullDecodeCalled := false
	image.RegisterFormat("oversized-poster-test", "OVERSIZED",
		func(io.Reader) (image.Image, error) {
			fullDecodeCalled = true
			return nil, errors.New("full decode called")
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{Width: 8_192, Height: 8_192}, nil
		},
	)

	if _, err := decodePoster([]byte("OVERSIZED")); err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("excessive poster dimensions returned %v", err)
	}
	if fullDecodeCalled {
		t.Fatal("oversized poster reached full decode")
	}
}
