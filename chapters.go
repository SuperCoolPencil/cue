package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/mediaserver"
	"github.com/SuperCoolPencil/cue/internal/segments"
)

func runChapters(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("chapters", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var item, file string
	var write bool
	fs.StringVar(&item, "item", "", "server item ID whose segments should be exported")
	fs.StringVar(&file, "file", "", "writable local original MKV (required with --write)")
	fs.BoolVar(&write, "write", false, "embed chapters into --file, preserving a .cue-chapters.bak backup")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if item == "" || fs.NArg() != 0 || (write && file == "") {
		_, _ = fmt.Fprintln(stderr, "Usage: cue chapters --item <id> [--write --file <original.mkv>]")
		return 2
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	client, err := mediaserver.NewClient(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	media, err := client.ResolvePlayable(ctx, item)
	cancel()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Could not resolve selected media source")
		return 1
	}
	requestCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	metadata, metadataErr := client.GetMediaItem(requestCtx, item)
	stop()
	showID := ""
	if metadataErr == nil {
		showID = metadata.ShowID
	}
	if len(media.Segments) == 0 {
		identity := segments.Identity{Server: cfg.Server.URL, User: cfg.Server.UserID, Show: showID, Item: item, Source: media.SourceID, Revision: media.Revision, DurationMs: media.DurationMs}
		if cached, ok := segments.DefaultCache().Load(identity); ok {
			media.Segments = cached.Segments
		}
	}
	if !write {
		b, err := segments.ChapterXML(media.Segments, media.DurationMs)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if _, err := stdout.Write(b); err != nil {
			return 1
		}
		return 0
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := segments.EmbedMKV(ctx, file, media.Segments, media.DurationMs); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "Chapters embedded; original preserved as .cue-chapters.bak. Refresh server metadata if needed.")
	return 0
}
