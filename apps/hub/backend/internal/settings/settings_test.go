package settings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zertex-labs/kyoku-hub/packages/validation"
)

// A user who never opened settings must get the behaviour of the tagger before
// it was configurable: all the artwork work, none of the new requirements.
func TestDefaultsKeepTheOldBehaviour(t *testing.T) {
	d := Default()
	if !d.EmbedCover || !d.WriteCoverFile {
		t.Fatalf("defaults = %+v, want both artwork steps on", d)
	}
	if d.RequireMusicBrainzTags {
		t.Error("MusicBrainz ids must not be required by default")
	}
	if d.PreferProviderGenre {
		t.Error("MusicBrainz genre must win by default")
	}

	for _, tag := range d.RequiredTags() {
		if tag == validation.TagMBTrackID {
			t.Errorf("default required tags include %q", tag)
		}
	}
}

// Turning the option on must add the identifiers to what validation enforces,
// without dropping the core five.
func TestRequiringMusicBrainzTagsExtendsTheList(t *testing.T) {
	o := Default()
	o.RequireMusicBrainzTags = true

	tags := o.RequiredTags()
	for _, want := range append(validation.DefaultRequiredTags, validation.MusicBrainzTags...) {
		found := false
		for _, got := range tags {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required tags missing %q (got %v)", want, tags)
		}
	}
}

// A row written before an option existed reads back with the missing key at its
// default, not at false — otherwise adding an option would silently turn it off
// for everyone who had saved settings before.
func TestStoredOptionsMergeOverDefaults(t *testing.T) {
	raw := []byte(`{"requireMusicBrainzTags":true}`)

	out := Default()
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !out.RequireMusicBrainzTags {
		t.Error("stored value was ignored")
	}
	if !out.EmbedCover || !out.WriteCoverFile {
		t.Errorf("absent keys lost their default: %+v", out)
	}
}

// Static is what tests and pre-auth code paths use; it must ignore the user.
func TestStaticIgnoresTheUser(t *testing.T) {
	p := Static{EmbedCover: false}
	if got := p.Get(context.Background(), "anyone"); got.EmbedCover {
		t.Error("Static returned its own zero value instead of the configured one")
	}
}

// A nil store is a valid store: it means "not configured yet".
func TestNilStoreGivesDefaults(t *testing.T) {
	var s *Store
	if got := s.Get(context.Background(), "user"); got != Default() {
		t.Errorf("got %+v, want the defaults", got)
	}
}
