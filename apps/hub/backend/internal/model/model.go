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
