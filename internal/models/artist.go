package models

// ArtistInfo represents aggregated artist information from various sources
type ArtistInfo struct {
	Bio          string   // Artist biography text
	Source       string   // "discogs", "theaudiodb", "wikipedia"
	ThumbnailURL string   // Primary image URL
	GalleryURLs  []string // Secondary images (for future gallery view)
	Discography  string   // Newline-separated "Album (Year)" entries
	PageURL      string   // Link to source page
}
