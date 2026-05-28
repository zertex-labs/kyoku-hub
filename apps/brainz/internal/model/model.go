package model

import (
	"time"
)

type Artist struct {
	MBArtistID     string    `json:"mb_artist_id"`
	Name           string    `json:"name"`
	SortName       string    `json:"sort_name"`
	Disambiguation string    `json:"disambiguation"`
	Type           string    `json:"type"`
	Country        string    `json:"country"`
	ImageURL       string    `json:"image_url"`
	Description    string    `json:"description"`
	LastFetchedAt  time.Time `json:"last_fetched_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReleaseGroup struct {
	MBReleaseGroupID string `json:"mb_release_group_id"`
	MBArtistID       string `json:"mb_artist_id"`
	Title            string `json:"title"`
	PrimaryType      string `json:"primary_type"`
	SecondaryTypes   string `json:"secondary_types"`
	Disambiguation   string `json:"disambiguation"`
	FirstReleaseDate string `json:"first_release_date"`
	CreatedAt        string `json:"created_at"`
}

type Release struct {
	MBReleaseID      string    `json:"-"`
	MBReleaseGroupID string    `json:"-"`
	Title            string    `json:"title"`
	Status           string    `json:"status"`
	Country          string    `json:"country"`
	Date             string    `json:"date"`
	Barcode          string    `json:"barcode"`
	CreatedAt        time.Time `json:"created_at"`
}

type HubInstance struct {
	ID        string    `json:"id"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
}

type HubSubscriptions struct {
	HubInstanceID string    `json:"hub_instance_id"`
	MBArtistID    string    `json:"mb_artist_id"`
	SubscribedAt  time.Time `json:"subscribed_at"`
}

type ArtistRequest struct {
	MBArtistID    string    `json:"-"`
	HubInstanceID string    `json:"hub_instance_id"`
	RequestCount  int       `json:"request_count"`
	LastRequestAt time.Time `json:"last_request_at"`
}
