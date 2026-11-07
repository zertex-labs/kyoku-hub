package download

import (
	"context"
	"fmt"
)

// MBRelease is one edition of a release group, as the download page lists it.
//
// A group is the album; a release is the thing you own — the one with a label, a
// country and a date. 放生会 has seven on MusicBrainz, all thirteen tracks, and
// only the user knows which one they mean.
type MBRelease struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	Country    string `json:"country"`
	Status     string `json:"status"`
	TrackCount int    `json:"trackCount"`
}

// MBReleases lists the releases of a release group.
//
// One request, at the same 1/s MusicBrainz budget as everything else. A failure
// is not fatal anywhere: the download page falls back to "match by track count",
// which is what it did before the picker existed.
func (m *Manager) MBReleases(ctx context.Context, rgID string) ([]MBRelease, error) {
	if m.mb == nil {
		return nil, fmt.Errorf("musicbrainz unavailable")
	}
	if !looksLikeUUID(rgID) {
		return nil, fmt.Errorf("not a musicbrainz id: %s", rgID)
	}

	var group mbGroupResp
	if err := m.mb.get(ctx, "/release-group/"+rgID+"?fmt=json&inc=releases+media", &group); err != nil {
		// The caller decides how loud this is; a release list is optional.

		return nil, err
	}

	out := make([]MBRelease, 0, len(group.Releases))
	for _, r := range group.Releases {
		tracks := 0
		for _, m := range r.Media {
			tracks += m.TrackCount
		}
		out = append(out, MBRelease{
			ID:         r.ID,
			Title:      r.Title,
			Date:       r.Date,
			Country:    r.Country,
			Status:     r.Status,
			TrackCount: tracks,
		})
	}
	return out, nil
}
