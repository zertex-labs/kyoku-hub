package download

import "testing"

// The provider's genre is a regional label — Deezer returned "Asian Music" for
// Ado, and "Азиатска музика" for the same album from a Bulgarian IP. MusicBrainz
// has real genres per release group ("j-pop", "j-rock"), so those win, and the
// provider is only a fallback for groups MusicBrainz has none for.
func TestGenresPreferMusicBrainz(t *testing.T) {
	got := firstNonEmptyStrings([]string{"j-pop", "j-rock"}, "Asian Music")
	if len(got) != 2 || got[0] != "j-pop" || got[1] != "j-rock" {
		t.Fatalf("genres = %v, want the MusicBrainz ones", got)
	}

	// Nothing from MusicBrainz: a coarse genre beats no genre.
	got = firstNonEmptyStrings(nil, "Asian Music")
	if len(got) != 1 || got[0] != "Asian Music" {
		t.Fatalf("fallback = %v", got)
	}

	if got := firstNonEmptyStrings(nil, ""); got != nil {
		t.Fatalf("empty input produced %v", got)
	}
}
