// Package plugin-api is the wire contract between the hub and a plugin: the
// hook names, the payload a plugin receives, and the answer it gives back.
//
// It is a package of its own so both sides depend on one definition. A plugin is
// a separate Go module; if the shapes lived in the hub the SDK could only copy
// them, and the copy would drift.
package api

// APIVersion is the contract version. It is declared in the manifest, echoed by
// the binary on --describe, and refused on mismatch: a payload shape change is a
// breaking change, not a silent one.
const APIVersion = 1

// Hook is a point in a download where plugins are invited to run.
type Hook string

const (
	// AlbumProbed fires once the source has been classified, before any bytes
	// move. Read-only.
	AlbumProbed Hook = "album.probed"
	// TrackDownloaded fires per file, before kyoku's own tagging.
	TrackDownloaded Hook = "track.downloaded"
	// TrackTagged fires per file, after tagging and artwork.
	TrackTagged Hook = "track.tagged"
	// AlbumBeforePublish fires once with every staged file, before the album
	// moves into the library. Files a plugin adds here travel with the album.
	AlbumBeforePublish Hook = "album.beforePublish"
	// AlbumPublished fires once the album is in the library.
	AlbumPublished Hook = "album.published"
)

// KnownHooks is the set the hub dispatches, used to validate manifests.
var KnownHooks = []Hook{AlbumProbed, TrackDownloaded, TrackTagged, AlbumBeforePublish, AlbumPublished}

func (h Hook) Known() bool {
	for _, k := range KnownHooks {
		if k == h {
			return true
		}
	}
	return false
}

// IsTrackHook reports whether the hook runs once per file.
func (h Hook) IsTrackHook() bool {
	return h == TrackDownloaded || h == TrackTagged
}

// App identifies the host.
type App struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Album is the album-level part of the payload.
type Album struct {
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	Year           string `json:"year"`
	Genre          string `json:"genre"`
	Label          string `json:"label"`
	ReleaseType    string `json:"releaseType"`
	ReleaseGroupID string `json:"releaseGroupId"`
	Dir            string `json:"dir"`
	TrackCount     int    `json:"trackCount"`
	CoverPath      string `json:"coverPath,omitempty"`
}

// Track is the per-file part of the payload.
type Track struct {
	Path       string `json:"path"`
	Ext        string `json:"ext"`
	Title      string `json:"title"`
	Number     int    `json:"number"`
	TrackTotal int    `json:"trackTotal"`
	Disc       int    `json:"disc"`
	DiscTotal  int    `json:"discTotal"`
	ISRC       string `json:"isrc,omitempty"`
	MBTrackID  string `json:"musicbrainzTrackId,omitempty"`
	MBArtistID string `json:"musicbrainzArtistId,omitempty"`
	MBAlbumID  string `json:"musicbrainzAlbumId,omitempty"`
}

// File is one staged file, for album hooks.
type File struct {
	Path string `json:"path"`
	Ext  string `json:"ext"`
}

// Payload is what a plugin receives on stdin.
type Payload struct {
	Hook   string         `json:"hook"`
	Plugin string         `json:"plugin"`
	App    App            `json:"app"`
	Album  Album          `json:"album"`
	Track  *Track         `json:"track,omitempty"`
	Files  []File         `json:"files,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

// Response is the optional JSON a plugin may print on stdout.
type Response struct {
	Message string   `json:"message,omitempty"`
	Files   []string `json:"files,omitempty"`
}

// Describe is what `--describe` prints: enough for the hub to verify the binary
// against the manifest before it is ever granted a single permission.
type Describe struct {
	APIVersion int      `json:"apiVersion"`
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Hooks      []string `json:"hooks"`
}

// Exit codes. Skip is not a failure: the plugin had nothing to do.
const (
	ExitOK    = 0
	ExitError = 1
	ExitSkip  = 3
)
