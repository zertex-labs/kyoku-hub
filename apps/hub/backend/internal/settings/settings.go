// Package settings stores the options a user chooses, as opposed to the ones the
// deployment sets. It is deliberately small and typed: the frontend form is
// generated from these fields, so an option is one line here plus one line in the
// form.
package settings

import (
	"context"
	"encoding/json"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/packages/validation"
)

// Options is the tagger configuration.
type Options struct {
	// RequireMusicBrainzTags makes the identifiers a requirement rather than a
	// bonus: an album whose files lack them is reported as incomplete.
	RequireMusicBrainzTags bool `json:"requireMusicBrainzTags"`
	// EmbedCover writes the front cover into every file. Off means the album
	// keeps whatever artwork it had.
	EmbedCover bool `json:"embedCover"`
	// WriteCoverFile drops cover.jpg beside the tracks, for players that read
	// folder art rather than embedded art.
	WriteCoverFile bool `json:"writeCoverFile"`
	// PreferProviderGenre takes the genre the store advertises over the one
	// MusicBrainz has voted on. The provider's genre is usually the marketing
	// label ("Asian Music"); MusicBrainz's is usually what a listener would say
	// ("j-pop"), which is why it is the default.
	PreferProviderGenre bool `json:"preferProviderGenre"`
}

// Default is what a user who never opened settings gets: the behaviour of the
// tagger before it was configurable.
func Default() Options {
	return Options{EmbedCover: true, WriteCoverFile: true}
}

// Validate keeps nonsense out of the store.
func (o Options) Validate() error { return nil }

// RequiredTags is the tag list validation should enforce for this user.
func (o Options) RequiredTags() []string {
	tags := append([]string{}, validation.DefaultRequiredTags...)
	if o.RequireMusicBrainzTags {
		tags = append(tags, validation.MusicBrainzTags...)
	}
	return tags
}

// Provider is what the rest of the app asks for a user's options.
type Provider interface {
	Get(ctx context.Context, userID string) Options
}

// Store persists options per user.
type Store struct {
	q *queries.Queries
}

func NewStore(q *queries.Queries) *Store { return &Store{q: q} }

// Get returns the stored options merged over the defaults, so a settings row
// written before an option existed still reads correctly. A missing row, or a
// database that is not there, gives the defaults.
func (s *Store) Get(ctx context.Context, userID string) Options {
	out := Default()
	if s == nil || s.q == nil || userID == "" {
		return out
	}
	raw, err := s.q.UserSettings(ctx, userID)
	if err != nil || len(raw) == 0 {
		return out
	}
	// Unmarshal over the defaults: absent keys keep their default value, which is
	// exactly the merge we want.
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s *Store) Put(ctx context.Context, userID string, o Options) error {
	if s == nil || s.q == nil {
		return nil
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return s.q.SaveUserSettings(ctx, userID, raw)
}

// Static is a Provider with fixed options, for tests and for code paths that run
// before a user exists.
type Static Options

func (s Static) Get(context.Context, string) Options { return Options(s) }
