package musicbrainz

type ArtistSearchResult struct {
	Created string        `json:"created"`
	Count   int           `json:"count"`
	Offset  int           `json:"offset"`
	Artists []ArtistMatch `json:"artists"`
}

type ArtistMatch struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SortName       string `json:"sort-name"`
	Disambiguation string `json:"disambiguation"`
	Score          int    `json:"score"`
	Type           string `json:"type"`
	Country        string `json:"country"`
}

type Artist struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	SortName       string         `json:"sort-name"`
	Disambiguation string         `json:"disambiguation"`
	Type           string         `json:"type"`
	Country        string         `json:"country"`
	ReleaseGroups  []ReleaseGroup `json:"release-groups,omitempty"`
	Relations      []Relation     `json:"relations,omitempty"`
}

type Relation struct {
	Type       string      `json:"type"`
	TargetType string      `json:"target-type"`
	URL        RelationURL `json:"url"`
}

type RelationURL struct {
	Resource string `json:"resource"`
}

type ReleaseGroup struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	PrimaryType      string   `json:"primary-type"`
	SecondaryTypes   []string `json:"secondary-types"`
	Disambiguation   string   `json:"disambiguation"`
	FirstReleaseDate string   `json:"first-release-date"`
}
