package plugins

import "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"

// The hook taxonomy and the wire payload belong to the download package: it is
// the producer. Aliasing them here keeps plugin code reading naturally
// (plugins.HookTrackTagged) without the download pipeline ever importing this
// package.
type (
	Hook    = download.Hook
	Payload = download.Payload
	App     = download.App
	Album   = download.Album
	Track   = download.Track
	File    = download.File
	Outcome = download.Outcome
)

const (
	HookAlbumProbed        = download.HookAlbumProbed
	HookTrackDownloaded    = download.HookTrackDownloaded
	HookTrackTagged        = download.HookTrackTagged
	HookAlbumBeforePublish = download.HookAlbumBeforePublish
	HookAlbumPublished     = download.HookAlbumPublished
)
