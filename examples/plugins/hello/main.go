// hello is the smallest useful plugin: it exercises both hook shapes, reads
// typed config, writes one file and reports a line to the job log.
package main

import (
	"context"
	"os"
	"path/filepath"

	kyoku "github.com/zertex-labs/kyoku-hub/packages/plugin-sdk"
)

func main() {
	kyoku.Serve(kyoku.Plugin{
		Name:    "hello",
		Version: "1.0.0",
		Registry: kyoku.Registry{
			TrackTagged:        track,
			AlbumBeforePublish: album,
		},
	})
}

type config struct {
	Greeting  string `json:"greeting"`
	WriteFile bool   `json:"writeFile"`
}

func track(ctx context.Context, ev *kyoku.Event) error {
	var cfg config
	if err := ev.Config(&cfg); err != nil {
		return err
	}
	if ev.Track == nil {
		return kyoku.Skip("no track in payload")
	}
	ev.Message("%s: %s", cfg.Greeting, ev.Track.Title)
	return nil
}

func album(ctx context.Context, ev *kyoku.Event) error {
	var cfg config
	if err := ev.Config(&cfg); err != nil {
		return err
	}
	if !cfg.WriteFile {
		return kyoku.Skip("writeFile is off")
	}

	path := filepath.Join(ev.Album.Dir, "hello.txt")
	if err := os.WriteFile(path, []byte(cfg.Greeting+" from kyoku\n"), 0o644); err != nil {
		return err
	}
	// Telling the hub what we added saves it diffing the directory.
	ev.Added(path)
	ev.Message("%s · %d files staged", cfg.Greeting, len(ev.Files))
	return nil
}
