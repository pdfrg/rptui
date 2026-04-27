package models

// LidarrAlbumInfo represents an album's status in Lidarr
type LidarrAlbumInfo struct {
	InLidarr        bool
	Monitored       bool
	HasFiles        bool
	PercentOfTracks float64
}

// ArtistInfo represents aggregated artist information from various sources.
// Each field can come from a different source (per-item fallback).
type ArtistInfo struct {
	Bio           string   // Artist biography text
	BioSource     string   // "theaudiodb", "discogs", "wikipedia"
	ThumbnailURL  string   // Primary image URL
	ThumbSource   string   // "discogs", "theaudiodb", "wikipedia"
	GalleryURLs   []string // Secondary images (for future gallery view)
	GallerySource string   // "discogs", "theaudiodb"
	Discography   string   // Newline-separated "Album (Year)" entries
	DiscoSource   string   // "musicbrainz", "wikipedia"
	PageURL       string   // Link to source page

	// Album info from TADB (shown after discography)
	AlbumDescription string // Album description/blurb from searchalbum
	AlbumSource      string // "theaudiodb"

	// Lidarr integration status
	LidarrInLidarr   bool                       // artist exists in Lidarr
	LidarrMonitored  bool                       // artist is monitored in Lidarr
	LidarrArtistID   int                        // Lidarr's internal artist ID
	LidarrArtistName string                     // Lidarr's matched name
	LidarrError      string                     // error if lookup failed
	LidarrAlbums     map[string]LidarrAlbumInfo // album title -> status
	LidarrMBID       string                     // MusicBrainz ID (for opening Lidarr search)
}
