package cache

type KeyBuilder interface {
	CacheKey() string
}

func GetKeyFor(k KeyBuilder) string {
	return k.CacheKey()
}

// -- Artist (artist:{mbid})
type ArtistLookup struct {
	MBID string
}

func (a ArtistLookup) CacheKey() string {
	return "artist:" + a.MBID
}

// -- Release Groups for Artist (release-groups:{artist_mbid})
type ReleaseGroupsLookup struct {
	MBID string
}

func (r ReleaseGroupsLookup) CacheKey() string {
	return "release-groups:" + r.MBID
}

// -- Release Group (release-group:{mbid})
type ReleaseGroupLookup struct {
	MBID string
}

func (r ReleaseGroupLookup) CacheKey() string {
	return "release-group:" + r.MBID
}

// -- Releases (releases:{rg_mbid}) [all releases for a release group]
type ReleasesLookup struct {
	RG_MBID string
}

func (r ReleasesLookup) CacheKey() string {
	return "releases:" + r.RG_MBID
}
