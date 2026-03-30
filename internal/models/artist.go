package models

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
}
