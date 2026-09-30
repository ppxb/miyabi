package domain

// SourceID belongs to one provider; it is never a JavDB discovery ID implicitly.
type SourceID struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// ImageCandidate describes a source image's intended use before downloading it.
type ImageCandidate struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Role     string `json:"role"` // cover, poster, preview or avatar
}

// MovieMetadata is independent of ownership and media-file indexing.
type MovieMetadata struct {
	Detail MovieDetail      `json:"detail"`
	Images []ImageCandidate `json:"images"`
}

// MetadataSnapshot records the videos exported for a movie in one library source.
// Videos is a fingerprint of the video identities, names, locations, sizes, and hashes.
type MetadataSnapshot struct {
	AccountID     string `json:"account_id"`
	DirectoryID   string `json:"directory_id"`
	Videos        string `json:"videos"`
	PosterVersion int    `json:"poster_version,omitempty"`
}
