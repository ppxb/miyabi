package domain

// MetadataSnapshot records the videos and sidecars exported for a movie in one library source.
// Videos is a fingerprint of the video identities, names, locations, sizes, and hashes.
type MetadataSnapshot struct {
	AccountID   string              `json:"account_id"`
	DirectoryID string              `json:"directory_id"`
	Videos      string              `json:"videos"`
	Directories []DirectorySnapshot `json:"directories,omitempty"`
	LocalExport bool                `json:"local_export,omitempty"`
}

// DirectorySnapshot records the sidecar state of a media directory.
type DirectorySnapshot struct {
	ID     string  `json:"id"`
	NFO    Sidecar `json:"nfo"`
	Poster Sidecar `json:"poster"`
	Fanart Sidecar `json:"fanart"`
}

// Sidecar identifies an NFO or artwork file by name and content hash.
type Sidecar struct {
	Name string `json:"name"`
	SHA1 string `json:"sha1"`
}
