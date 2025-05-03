package main

import (
	"Qischer/cs348-proj/model"
	"Qischer/cs348-proj/view/index"
	"Qischer/cs348-proj/view/layout"
	"context"
	"encoding/json"
	"strconv"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func indexFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)

	var tracks []model.Track
	var playlists []model.Playlist

	db.Joins("Artist").Find(&tracks)
	db.Find(&playlists)

	err := layout.Layout(index.Index(playlists, tracks)).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}
}

func getPlaylistFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)

	var playlists []model.Playlist
	db.Find(&playlists)

	pid, err := strconv.Atoi(req.PathValue("id"))
	if err != nil {
		panic(err)
	}

	var p model.Playlist
	db.First(&p, pid)

	var tracks []model.Track

	db.Raw(`
    SELECT tracks.* 
    FROM tracks
    JOIN track_playlist ON tracks.id = track_playlist.track_id
    JOIN artists ON artists.id = tracks.artist_id
    WHERE track_playlist.playlist_id = ?
    `, pid).Scan(&tracks)

	err = layout.Layout(index.PlaylistPage(playlists, p, tracks)).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}
}

func addToPlaylistFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)

	decoder := json.NewDecoder(req.Body)

	var body map[string]interface{}
	err := decoder.Decode(&body)
	if err != nil {
		panic(err)
	}

	fmt.Println(body)

	var track model.Track
	db.Find(&track, body["trackId"])

	var playlist model.Playlist
	db.Find(&playlist, body["playlistId"])

	track.Playlists = append(track.Playlists, playlist)
	db.Save(&track)
}

func createPlaylistFunc(res http.ResponseWriter, req *http.Request) {
	err := layout.Layout(index.CreatePlaylist()).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}
}

func editPlaylistFormFunc(res http.ResponseWriter, req *http.Request) {
	pid, err := strconv.Atoi(req.PathValue("id"))
	if err != nil {
		panic(err)
	}

	err = layout.Layout(index.UpdatePlaylist(pid)).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}
}

func deletePlaylistFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)
	pid, err := strconv.Atoi(req.PathValue("id"))
	if err != nil {
		panic(err)
	}

	var p model.Playlist
	db.First(&p, pid)

	db.Delete(&p)
}

func submitFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)
	err := req.ParseForm()
	if err != nil {
		panic(err)
	}

	p := model.Playlist{
		Name:        req.FormValue("name"),
		Description: req.FormValue("description"),
	}

	db.Create(&p)
	err = layout.Layout(index.CreatePlaylist()).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}

	fmt.Println("inserted new playlist")
}

func submitEditFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)
	err := req.ParseForm()
	if err != nil {
		panic(err)
	}

	pid, err := strconv.Atoi(req.PathValue("id"))
	if err != nil {
		panic(err)
	}

	var p model.Playlist
	db.First(&p, pid)
	p.Name = req.FormValue("name")
	p.Description = req.FormValue("description")
	db.Save(&p)

	err = layout.Layout(index.CreatePlaylist()).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}

	fmt.Println("inserted new playlist")
}

func tracksReportFunc(res http.ResponseWriter, req *http.Request) {
	db, _ := req.Context().Value("DB").(*gorm.DB)

	// Get all artists for the filter dropdown
	var artists []model.Artist
	db.Find(&artists)

	// Build the query with placeholders
	query := `
    SELECT t.*, a.name as artist_name, al.title as album_title
    FROM tracks t
    JOIN artists a ON t.artist_id = a.id
    JOIN albums al ON t.album_id = al.id
    WHERE 1=1
  `
	var args []interface{}

	if artistID := req.URL.Query().Get("artist"); artistID != "" {
		if id, err := strconv.Atoi(artistID); err == nil {
			query += " AND t.artist_id = ?"
			args = append(args, id)
		}
	}
	if albumID := req.URL.Query().Get("album"); albumID != "" {
		if id, err := strconv.Atoi(albumID); err == nil {
			query += " AND t.album_id = ?"
			args = append(args, id)
		}
	}
	if duration := req.URL.Query().Get("duration"); duration != "" {
		if minutes, err := strconv.Atoi(duration); err == nil {
			query += " AND t.duration <= ?"
			args = append(args, minutes*60)
		}
	}

	// Execute the prepared statement
	var tracks []struct {
		model.Track
		ArtistName string `gorm:"column:artist_name"`
		AlbumTitle string `gorm:"column:album_title"`
	}

	db.Raw(query, args...).Scan(&tracks)

	// Convert to the expected format
	var resultTracks []model.Track
	for _, t := range tracks {
		track := t.Track
		track.Artist = model.Artist{Name: t.ArtistName}
		track.Album = model.Album{Title: t.AlbumTitle}
		resultTracks = append(resultTracks, track)
	}

	err := layout.Layout(index.TracksReport(resultTracks, artists)).Render(req.Context(), res)
	if err != nil {
		panic(err)
	}
}

func startServer(router *http.ServeMux) {
	fmt.Println("Serving running in http://localhost:8080")

	server := http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	err := server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func DBMiddlewareHandler(db *gorm.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		timeoutContext, _ := context.WithTimeout(context.Background(), time.Second)
		ctx := context.WithValue(req.Context(), "DB", db.WithContext(timeoutContext))
		next.ServeHTTP(res, req.WithContext(ctx))
	})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		db := initDB()
		// Ensure the database is properly initialized
		var count int64
		db.Model(&model.Track{}).Count(&count)
		fmt.Printf("Database initialized successfully with %d tracks\n", count)
		return
	}

	router := http.NewServeMux()

	db := initDB()

	indexHandler := http.HandlerFunc(indexFunc)
	createPlaylistHandler := http.HandlerFunc(createPlaylistFunc)
	submitHandler := http.HandlerFunc(submitFunc)
	submitEditHandler := http.HandlerFunc(submitEditFunc)
	playlistPageHandler := http.HandlerFunc(getPlaylistFunc)
	addToPlaylistHandler := http.HandlerFunc(addToPlaylistFunc)
	tracksReportHandler := http.HandlerFunc(tracksReportFunc)

	editFormHandler := http.HandlerFunc(editPlaylistFormFunc)
	deletePlaylistHandler := http.HandlerFunc(deletePlaylistFunc)

	router.Handle("GET /", DBMiddlewareHandler(db, indexHandler))
	router.Handle("GET /p/{id}", DBMiddlewareHandler(db, playlistPageHandler))
	router.Handle("GET /create-playlist", DBMiddlewareHandler(db, createPlaylistHandler))
	router.Handle("GET /edit-playlist/{id}", DBMiddlewareHandler(db, editFormHandler))
	router.Handle("GET /tracks-report", DBMiddlewareHandler(db, tracksReportHandler))

	router.Handle("POST /submit-playlist", DBMiddlewareHandler(db, submitHandler))
	router.Handle("POST /update-playlist/{id}", DBMiddlewareHandler(db, submitEditHandler))

	router.Handle("PUT /add-to-playlist", DBMiddlewareHandler(db, addToPlaylistHandler))

	router.Handle("DELETE /delete-playlist/{id}", DBMiddlewareHandler(db, deletePlaylistHandler))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go startServer(router)

	<-c
	fmt.Println("Shutting down server")
}

func populateTracks(db *gorm.DB) {
	// Create mock albums and artists
	albums := []model.Album{
		{Title: "Thriller"},
		{Title: "Abbey Road"},
		{Title: "Back in Black"},
		{Title: "Rumours"},
	}

	artists := []model.Artist{
		{Name: "Michael Jackson"},
		{Name: "The Beatles"},
		{Name: "AC/DC"},
		{Name: "Fleetwood Mac"},
	}

	for i := range albums {
		db.FirstOrCreate(&albums[i], model.Album{Title: albums[i].Title})
	}

	for i := range artists {
		db.FirstOrCreate(&artists[i], model.Artist{Name: artists[i].Name})
	}

	// Create mock tracks
	tracks := []model.Track{
		{Title: "Billie Jean", Duration: 294, AlbumId: albums[0].ID, ArtistId: artists[0].ID},
		{Title: "Beat It", Duration: 258, AlbumId: albums[0].ID, ArtistId: artists[0].ID},
		{Title: "Thriller", Duration: 357, AlbumId: albums[0].ID, ArtistId: artists[0].ID},
		{Title: "Come Together", Duration: 259, AlbumId: albums[1].ID, ArtistId: artists[1].ID},
		{Title: "Something", Duration: 182, AlbumId: albums[1].ID, ArtistId: artists[1].ID},
		{Title: "Back in Black", Duration: 255, AlbumId: albums[2].ID, ArtistId: artists[2].ID},
		{Title: "You Shook Me All Night Long", Duration: 210, AlbumId: albums[2].ID, ArtistId: artists[2].ID},
		{Title: "Go Your Own Way", Duration: 216, AlbumId: albums[3].ID, ArtistId: artists[3].ID},
		{Title: "Dreams", Duration: 257, AlbumId: albums[3].ID, ArtistId: artists[3].ID},
	}

	for _, track := range tracks {
		db.FirstOrCreate(&track, model.Track{Title: track.Title})
	}
}

func initDB() *gorm.DB {
	fmt.Println("Initializing database")
	db, err := gorm.Open(sqlite.Open("core.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect to database")
	}

	db.AutoMigrate(&model.Track{})
	db.AutoMigrate(&model.Playlist{})

	populateTracks(db)
	fmt.Println("Populated data")

	return db
}
