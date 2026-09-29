package domain

// MetadataSnapshot records the videos exported for a movie in one library source.
// Videos is a fingerprint of the video identities, names, locations, sizes, and hashes.
type MetadataSnapshot struct {
	AccountID   string `json:"account_id"`
	DirectoryID string `json:"directory_id"`
	Videos      string `json:"videos"`
}
