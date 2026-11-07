// replaygain writes ReplayGain 2.0 tags into a downloaded album with rsgain.
//
// It runs at album.beforePublish: kyoku has already written its own tags, and
// rsgain's REPLAYGAIN_* keys are preserved because the hub only rewrites the
// keys it owns. Because it runs on the whole album directory, files get
// REPLAYGAIN_ALBUM_GAIN as well as the per-track values.
package main

import (
	"context"
	"fmt"
	"regexp"

	kyoku "github.com/zertex-labs/kyoku-hub/packages/plugin-sdk"
)

func main() {
	kyoku.Serve(kyoku.Plugin{
		Name:    "replaygain",
		Version: "2.0.0",
		Registry: kyoku.Registry{
			AlbumBeforePublish: replaygain,
		},
	})
}

// config mirrors plugin.yaml. The hub sends values already typed and merged with
// the manifest defaults, so a wrong field here is a manifest bug, not a runtime
// guess.
type config struct {
	Multithread       string `json:"multithread"`
	SkipExisting      bool   `json:"skipExisting"`
	ReferenceLoudness int    `json:"referenceLoudness"`
}

// rsgain's summary ends with a line like "Average Gain:      -13.83 dB".
var averageGain = regexp.MustCompile(`Average Gain:\s+(-?[0-9.]+) dB`)

func replaygain(ctx context.Context, ev *kyoku.Event) error {
	var cfg config
	if err := ev.Config(&cfg); err != nil {
		return err
	}

	args := []string{"easy", "-m", cfg.Multithread}
	if cfg.SkipExisting {
		// Idempotent: a retried or resumed download does not rescan.
		args = append(args, "-S")
	}
	if cfg.ReferenceLoudness != 0 {
		args = append(args, "-l", fmt.Sprint(cfg.ReferenceLoudness))
	}
	args = append(args, ev.Album.Dir)

	out, err := ev.Run(ctx, "rsgain", args...)
	if err != nil {
		return fmt.Errorf("rsgain: %w", err)
	}

	if m := averageGain.FindStringSubmatch(out); m != nil {
		ev.Message("album gain %s dB · %d track(s)", m[1], ev.Album.TrackCount)
		return nil
	}
	ev.Message("scanned %d track(s)", ev.Album.TrackCount)
	return nil
}
