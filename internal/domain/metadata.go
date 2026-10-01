package domain

// SourceID belongs to one provider; it is never a JavDB discovery ID implicitly.
type SourceID struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// CoverLayout distinguishes a single composition from a front/back jacket.
type CoverLayout string

const (
	CoverSingle CoverLayout = "single"
	CoverJacket CoverLayout = "jacket"
)

// ImageCandidate describes a source image's intended use before downloading it.
type ImageCandidate struct {
	Provider string      `json:"provider" xml:"provider,attr"`
	URL      string      `json:"url" xml:",chardata"`
	Role     string      `json:"role" xml:"role,attr"` // cover, poster, preview or avatar
	Layout   CoverLayout `json:"layout,omitempty" xml:"layout,attr,omitempty"`
	Width    int         `json:"width,omitempty" xml:"width,attr,omitempty"`
	Height   int         `json:"height,omitempty" xml:"height,attr,omitempty"`
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
