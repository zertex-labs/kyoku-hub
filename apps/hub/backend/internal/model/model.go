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

type Artist struct {
	MBArtistID        string     `json:"mb_artist_id"`
	Name              string     `json:"name"`
	Disambiguation    string     `json:"disambiguation"`
	ImageURL          string     `json:"image_url"`
	Description       string     `json:"description"`
	ReleaseGroupCount int        `json:"release_group_count"`
	Status            string     `json:"status"`
	Active            bool       `json:"active"`
	CreatedAt         time.Time  `json:"created_at"`
	SyncedAt          *time.Time `json:"synced_at,omitempty"`
}
