package download

import (
	"context"
	"fmt"
)

// MBAliases returns the other-language names of a release group.
//
// This is the bridge the search is missing: MusicBrainz calls Ado's single ルル
// and records "RuLe" as its English alias, while Deezer stocks it as "RuLe". No
// amount of string comparison across two scripts finds that pair, but the
// database already knows they are the same release.
func (m *Manager) MBAliases(ctx context.Context, rgID string) ([]string, error) {
	if m.mb == nil {
		return nil, fmt.Errorf("musicbrainz unavailable")
	}
	if !looksLikeUUID(rgID) {
		return nil, fmt.Errorf("not a musicbrainz id: %s", rgID)
	}

	var group struct {
		Aliases []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			Locale string `json:"locale"`
		} `json:"aliases"`
	}
	if err := m.mb.get(ctx, "/release-group/"+rgID+"?fmt=json&inc=aliases", &group); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(group.Aliases))
	for _, a := range group.Aliases {
		if a.Name != "" {
			out = append(out, a.Name)
		}
	}
	return out, nil
}
