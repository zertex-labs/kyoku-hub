package download

import (
	"context"

	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"
)

// The hook taxonomy and the wire payload are defined in packages/plugin-api so
// that a plugin, its SDK and the hub all agree on one definition.
type (
	Hook    = api.Hook
	Payload = api.Payload
	App     = api.App
	Album   = api.Album
	Track   = api.Track
	File    = api.File
)

const (
	HookAlbumProbed        = api.AlbumProbed
	HookTrackDownloaded    = api.TrackDownloaded
	HookTrackTagged        = api.TrackTagged
	HookAlbumBeforePublish = api.AlbumBeforePublish
	HookAlbumPublished     = api.AlbumPublished
)

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
