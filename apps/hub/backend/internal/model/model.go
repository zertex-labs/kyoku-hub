package model

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	MBUserID  string    `json:"-"` // MusicBrainz user sub/id
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	ExpiresAt time.Time `json:"-"`
	CreatedAt time.Time `json:"-"`
}

type FsAgent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	Label     string    `json:"label"`
	PublicKey string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type ReleaseGroupState string

const (
	RGStatePerfect ReleaseGroupState = "PERFECT"
	RGStateMBMiss  ReleaseGroupState = "MB_MISS"
	RGStateBad     ReleaseGroupState = "BAD"
	RGStateMissing ReleaseGroupState = "MISSING"
)

type Artist struct {
	MBArtistID        string     `json:"mb_artist_id"`
	Name              string     `json:"name"`
	Disambiguation    string     `json:"disambiguation"`
	ImageURL          string     `json:"image_url"`
	Description       string     `json:"description"`
	ReleaseGroupCount int        `json:"release_group_count"`
	Status            string     `json:"status"`
	Active            bool       `json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	SyncedAt          *time.Time `json:"synced_at,omitempty"`
}
