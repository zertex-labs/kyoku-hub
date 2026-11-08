package download

import (
	"testing"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// genres decides whose vocabulary ends up in the file. The provider's is a
// marketing bucket and MusicBrainz's is what a listener would say, so MB wins
// unless the user says otherwise.
func TestGenrePreference(t *testing.T) {
	mb := &mbInfo{groupGenres: []string{"j-pop"}}
	cand := downloader.AlbumCandidate{Genre: "Asian Music"}
	job := &Job{}

	m := NewManager(t.TempDir(), nil, nil, nil, nil)

	m.SetSettings(settings.Static{})
	if got := m.genres(job, mb, cand); len(got) == 0 || got[0] != "j-pop" {
		t.Errorf("default genres = %v, want the MusicBrainz one first", got)
	}

	m.SetSettings(settings.Static{PreferProviderGenre: true})
	if got := m.genres(job, mb, cand); len(got) == 0 || got[0] != "Asian Music" {
		t.Errorf("configured genres = %v, want the provider one first", got)
	}
}

// With no user store the manager must behave as it always did.
func TestNoSettingsProviderMeansDefaults(t *testing.T) {
	m := NewManager(t.TempDir(), nil, nil, nil, nil)
	if got := m.options(&Job{}); got != settings.Default() {
		t.Fatalf("options = %+v, want the defaults", got)
	}
	if names := m.chain(settings.Default()).Names(); len(names) != 2 || names[0] != "tags" || names[1] != "cover" {
		t.Errorf("default chain = %v, want tags and cover", names)
	}
}

// Turning artwork off must remove the processor instead of leaving one that skips.
func TestArtworkOffDropsTheProcessor(t *testing.T) {
	m := NewManager(t.TempDir(), nil, nil, nil, nil)
	names := m.chain(settings.Options{EmbedCover: false, WriteCoverFile: false}).Names()
	if len(names) != 1 || names[0] != "tags" {
		t.Fatalf("chain = %v, want tags only", names)
	}
}
