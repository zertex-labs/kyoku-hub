// Package kyoku is the plugin SDK: it owns the wire contract so a plugin author
// never writes JSON, exit codes or stdin handling.
//
// A plugin is a normal Go program with a main():
//
//	func main() {
//		kyoku.Serve(kyoku.Plugin{
//			Name: "replaygain", Version: "2.0.0",
//			Registry: kyoku.Registry{AlbumBeforePublish: replaygain},
//		})
//	}
//
// Serve prints the describe document for --describe, otherwise reads one payload
// from stdin, calls the handler for that hook, writes the response and exits
// with the code the hub expects.
package kyoku

import (
	"context"
	"fmt"
)

// Registry maps hooks to handlers. Only the hooks you fill in are advertised by
// --describe, and the hub refuses a plugin whose manifest declares anything the
// binary does not implement.
type Registry struct {
	AlbumProbed        func(context.Context, *Event) error
	TrackDownloaded    func(context.Context, *Event) error
	TrackTagged        func(context.Context, *Event) error
	AlbumBeforePublish func(context.Context, *Event) error
	AlbumPublished     func(context.Context, *Event) error
}

// Plugin is what Serve runs.
type Plugin struct {
	Name     string
	Version  string
	Registry Registry
}

func (p Plugin) handlerFor(hook string) (func(context.Context, *Event) error, bool) {
	switch hook {
	case "album.probed":
		return p.Registry.AlbumProbed, p.Registry.AlbumProbed != nil
	case "track.downloaded":
		return p.Registry.TrackDownloaded, p.Registry.TrackDownloaded != nil
	case "track.tagged":
		return p.Registry.TrackTagged, p.Registry.TrackTagged != nil
	case "album.beforePublish":
		return p.Registry.AlbumBeforePublish, p.Registry.AlbumBeforePublish != nil
	case "album.published":
		return p.Registry.AlbumPublished, p.Registry.AlbumPublished != nil
	}
	return nil, false
}

// hooks lists what this plugin implements, in the order the hub dispatches them.
func (p Plugin) hooks() []string {
	out := []string{}
	for _, h := range []string{"album.probed", "track.downloaded", "track.tagged", "album.beforePublish", "album.published"} {
		if _, ok := p.handlerFor(h); ok {
			out = append(out, h)
		}
	}
	return out
}

// SkipError asks the hub to record the run as skipped rather than failed. Return
// Skip("nothing to do") — it is not an error the user should have to read.
type skipError struct{ reason string }

func (s skipError) Error() string { return s.reason }

func Skip(reason string) error { return skipError{reason} }

func isSkip(err error) (string, bool) {
	if s, ok := err.(skipError); ok {
		return s.reason, true
	}
	return "", false
}

// Errorf is fmt.Errorf with the plugin's name prefixed, so a job log line reads
// like the plugin said it.
func (p Plugin) Errorf(format string, a ...any) error {
	return fmt.Errorf(p.Name+": "+format, a...)
}
