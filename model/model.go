package model

import (
	"gorm.io/gorm"
)

type Track struct {
	gorm.Model
	Title    string
	Duration int

	AlbumId int
	Album   Album

	ArtistId int
	Artist   Artist

	Playlists []Playlist `gorm:"many2many:track_playlist;"`
}

type Album struct {
	ID    int
	Title string
}

type Artist struct {
	ID   int
	Name string
}

type Playlist struct {
	gorm.Model
	Name        string
	Description string
}

type TrackFilter struct {
	ArtistID    int
	AlbumID     int
	MaxDuration int
}
