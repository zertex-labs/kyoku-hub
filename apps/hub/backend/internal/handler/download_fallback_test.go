package handler

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// artistOnly is a source that cannot search but can list a discography — the
// shape the fallback exists for.
type artistOnly struct {
	albums []downloader.SearchResult
	err    error
	called bool
}

func (artistOnly) Name() string                     { return "fake" }
func (artistOnly) Ceiling() downloader.AudioQuality { return downloader.UnknownQuality }
func (artistOnly) SearchAlbums(context.Context, string) ([]downloader.SearchResult, error) {
	return nil, nil
}
func (fake artistOnly) ProbeAlbum(context.Context, string, downloader.AlbumRef, []downloader.QualityTarget) (downloader.AlbumCandidate, error) {
	return downloader.AlbumCandidate{}, nil
}
func (fake artistOnly) DownloadTrack(context.Context, string, string, string) (downloader.AudioQuality, error) {
	return downloader.UnknownQuality, nil
}

func (f *artistOnly) ArtistAlbums(context.Context, string, string) ([]downloader.SearchResult, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return f.albums, nil
}

// The real case: Deezer indexes 椎名林檎 as "Sheena Ringo" and 三毒史 as
// "triviṣa-itihāsa". Nothing matches the query text, and a name comparison
// across scripts would throw the album away — so the fallback must return the
// discography untouched.
func TestArtistFallbackKeepsResultsUnderAnotherName(t *testing.T) {
	src := &artistOnly{albums: []downloader.SearchResult{
		{ID: "97697302", Title: "triviṣa-itihāsa", Artist: "Sheena Ringo", Year: "2019"},
		{ID: "118144942", Title: "Apple Of Universal Gravity", Artist: "Sheena Ringo", Year: "2019"},
	}}

	got := artistFallback(context.Background(), map[string]downloader.Source{"fake": src}, "椎名林檎", "三毒史", zap.NewNop())
	if !src.called {
		t.Fatal("fallback never asked the source")
	}
	if len(got) != 2 {
		t.Fatalf("dropped results: %+v", got)
	}
	if got[0].Title != "triviṣa-itihāsa" {
		t.Fatalf("wrong order: %+v", got)
	}
}

// A source that errors must not take the whole search down with it.
func TestArtistFallbackSkipsFailingSource(t *testing.T) {
	bad := &artistOnly{err: errors.New("provider down")}
	good := &artistOnly{albums: []downloader.SearchResult{{ID: "1", Title: "Carnival"}}}

	got := artistFallback(context.Background(), map[string]downloader.Source{"bad": bad, "good": good}, "椎名林檎", "", zap.NewNop())
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("got %+v", got)
	}
}

// A source without the optional half is simply passed over.
func TestArtistFallbackIgnoresPlainSources(t *testing.T) {
	got := artistFallback(context.Background(), map[string]downloader.Source{"plain": fakeSource{}}, "x", "y", zap.NewNop())
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

type fakeSource struct{}

func (fakeSource) Name() string                     { return "plain" }
func (fakeSource) Ceiling() downloader.AudioQuality { return downloader.UnknownQuality }
func (fakeSource) SearchAlbums(context.Context, string) ([]downloader.SearchResult, error) {
	return nil, nil
}
func (fakeSource) ProbeAlbum(context.Context, string, downloader.AlbumRef, []downloader.QualityTarget) (downloader.AlbumCandidate, error) {
	return downloader.AlbumCandidate{}, nil
}
func (fakeSource) DownloadTrack(context.Context, string, string, string) (downloader.AudioQuality, error) {
	return downloader.UnknownQuality, nil
}

// The alias retry is triggered by "nothing matches the title", not by an empty
// result set: searching "Ado ルル" returns Ado's albums because the artist token
// matches, and every one of them is the wrong release.
func TestBestTitleRankDecidesWhetherToRetry(t *testing.T) {
	albums := []downloader.SearchResult{
		{Title: "Carnival"}, {Title: "Ado's Best Adobum"}, {Title: "KIRA"},
	}
	if got := bestTitleRank(albums, "ルル"); got >= 50 {
		t.Fatalf("rank = %d, a retry should be attempted", got)
	}

	exact := []downloader.SearchResult{{Title: "RuLe"}}
	if got := bestTitleRank(exact, "RuLe"); got < 50 {
		t.Fatalf("rank = %d, an exact title needs no retry", got)
	}
}
