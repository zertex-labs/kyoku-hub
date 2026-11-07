package handler

import (
	"testing"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// Real Deezer output for a release group search, the shape that made a picker
// show six artists for one album. Titles and artists are verbatim.
func TestRankResultsKeepsOnlyTheReleaseGroupArtist(t *testing.T) {
	items := []downloader.SearchResult{
		{ID: "1", Title: "DND", Artist: "Ado", TrackCount: 1, Provider: "deezer"},
		{ID: "2", Title: "Dark Things", Artist: "adonä", TrackCount: 7, Provider: "deezer"},
		{ID: "3", Title: "Around", Artist: "Noir", TrackCount: 8, Provider: "deezer"},
		{ID: "4", Title: "Adorn", Artist: "You'll Never Get To Heaven", TrackCount: 6, Provider: "deezer"},
		{ID: "5", Title: "Meau - Avond", Artist: "MEAU", TrackCount: 12, Provider: "deezer"},
		{ID: "6", Title: "Aduna", Artist: "Hommage", TrackCount: 10, Provider: "deezer"},
	}

	got := rankResults(items, "Ado", "DND")
	if len(got) != 1 {
		t.Fatalf("kept %d results: %+v", len(got), titles(got))
	}
	if got[0].Title != "DND" || got[0].Artist != "Ado" {
		t.Fatalf("kept the wrong release: %+v", got[0])
	}
}

// When the album has several editions, the plain one leads and remixes sink.
func TestRankResultsOrdersEditions(t *testing.T) {
	items := []downloader.SearchResult{
		{ID: "r", Title: "LEMONADE (Zedd Remix)", Artist: "aespa", Provider: "deezer"},
		{ID: "s", Title: "LEMONADE - The 2nd Album", Artist: "aespa", Provider: "deezer"},
		{ID: "d", Title: "LEMONADE (Deluxe Edition)", Artist: "aespa", Provider: "deezer"},
		{ID: "e", Title: "LEMONADE", Artist: "aespa", Provider: "deezer"},
	}

	got := rankResults(items, "aespa", "LEMONADE")
	if len(got) != 4 {
		t.Fatalf("kept %d, want all four: %+v", len(got), titles(got))
	}
	if got[0].Title != "LEMONADE" {
		t.Errorf("first = %q, want the exact title", got[0].Title)
	}
	if got[1].Title != "LEMONADE (Deluxe Edition)" {
		t.Errorf("second = %q, want the deluxe edition", got[1].Title)
	}
	if last := got[len(got)-1].Title; last != "LEMONADE (Zedd Remix)" {
		t.Errorf("last = %q, want the remix", last)
	}
}

// A search with no artist (old links, or a provider that does not report one)
// must not silently return an empty list.
func TestRankResultsIsSkippedWithoutAnArtist(t *testing.T) {
	items := []downloader.SearchResult{{ID: "1", Title: "DND", Artist: "Ado"}}
	if got := rankResults(items, "", ""); len(got) != len(items) {
		t.Fatalf("filtered without an artist: %+v", got)
	}
}

func titles(items []downloader.SearchResult) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Artist+" — "+it.Title)
	}
	return out
}
