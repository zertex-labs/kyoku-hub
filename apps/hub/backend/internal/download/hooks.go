package download

import "context"

// Hook is a point in a download where plugins are invited to run. The names live
// in this package because the download pipeline owns them: a plugin only
// subscribes, it never defines when it fires.
type Hook string

const (
	// HookAlbumProbed fires once the source has been classified, before any bytes
	// move. Read-only.
	HookAlbumProbed Hook = "album.probed"
	// HookTrackDownloaded fires per file, before kyoku's own tagging.
	HookTrackDownloaded Hook = "track.downloaded"
	// HookTrackTagged fires per file, after tagging and artwork.
	HookTrackTagged Hook = "track.tagged"
	// HookAlbumBeforePublish fires once with every staged file, before the album
	// moves into the library. Files a plugin adds here travel with the album.
	HookAlbumBeforePublish Hook = "album.beforePublish"
	// HookAlbumPublished fires once the album is in the library.
	HookAlbumPublished Hook = "album.published"
)

// App identifies the host in the payload.
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
	Hook   string `json:"hook"`
	Plugin string `json:"plugin"`
	App    App    `json:"app"`
	Album  Album  `json:"album"`
	Track  *Track `json:"track,omitempty"`
	Files  []File `json:"files,omitempty"`
}

// Outcome is the result of one plugin for one hook.
type Outcome struct {
	Plugin  string
	Hook    string
	Err     error
	Skipped bool
	Message string
	Stderr  string
}

// Dispatcher runs the plugins installed for a hook. The download pipeline only
// needs this contract: who implements it, and how plugins are confined, is not
// its business.
type Dispatcher interface {
	Dispatch(ctx context.Context, hook Hook, p Payload) []Outcome
	// Required reports whether the named plugin declared a failure as blocking.
	Required(name string) bool
	Names() []string
	Count() int
}
