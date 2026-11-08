package download

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
)

// The default has to be the shape an existing Picard library already uses,
// otherwise a download lands beside its own album instead of in it.
func TestAlbumDirDefaultIsPicardShape(t *testing.T) {
	got := AlbumDirFor(settings.DefaultAlbumPath, "aespa", "aespa", "Armageddon", 2024)
	want := filepath.Join("aespa", "[2024] Armageddon")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A missing year must take the brackets with it: "[{year}] {album}" is
// "[2023] Album" or "Album", never "[] Album".
func TestAlbumDirWithoutAYear(t *testing.T) {
	for _, template := range []string{"{artist}/[{year}] {album}", "{artist}/({year}) {album}"} {
		got := AlbumDirFor(template, "aespa", "aespa", "Welcome to MY World", 0)
		want := filepath.Join("aespa", "Welcome to MY World")
		if got != want {
			t.Errorf("%s: got %q, want %q", template, got, want)
		}
	}
}

// Templates are the user's: flat albums, year first, anything with {album}.
func TestAlbumDirHonoursCustomTemplates(t *testing.T) {
	cases := []struct{ template, want string }{
		{"{album}", "Armageddon"},
		{"{year} - {album}", "2024 - Armageddon"},
		{"{artist}/{album} ({year})", filepath.Join("aespa", "Armageddon (2024)")},
		{"{albumartist}/[{year}] {album}", filepath.Join("aespa", "[2024] Armageddon")},
	}
	for _, tc := range cases {
		if got := AlbumDirFor(tc.template, "aespa", "aespa", "Armageddon", 2024); got != tc.want {
			t.Errorf("%s → %q, want %q", tc.template, got, tc.want)
		}
	}
}

// A slash in a title must not invent a directory level.
func TestAlbumDirSanitisesEverySegment(t *testing.T) {
	got := AlbumDirFor("{artist}/{album}", "AC/DC", "AC/DC", "Back in Black", 1980)
	segments := strings.Split(got, string(filepath.Separator))
	if len(segments) != 2 {
		t.Fatalf("got %q (%d segments)", got, len(segments))
	}
}

// An empty template falls back to the default rather than producing a stray
// directory at the library root.
func TestAlbumDirEmptyTemplateUsesTheDefault(t *testing.T) {
	if got, want := AlbumDirFor("", "aespa", "aespa", "Armageddon", 2024),
		filepath.Join("aespa", "[2024] Armageddon"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
