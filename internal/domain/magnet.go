package domain

// Magnet describes a download resource for a movie. HasSubtitle and HD are
// the site's own labels; Tags adds labels inferred from the resource name and
// Inferred reports whether any tag came from inference rather than the site.
type Magnet struct {
	Hash        string   `json:"hash"`
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	HasSubtitle bool     `json:"has_subtitle"`
	HD          bool     `json:"hd"`
	FilesCount  int      `json:"files_count"`
	CreatedAt   string   `json:"created_at"`
	Sources     []string `json:"sources,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}

// Magnet tag vocabulary shared by every source and the frontend badges.
const (
	MagnetTagSubtitle   = "字幕"
	MagnetTagHD         = "高清"
	MagnetTag4K         = "4K"
	MagnetTagUncensored = "无码"
	MagnetTagCracked    = "破解"
)

// Magnet source names as they appear in Magnet.Sources.
const (
	MagnetSourceJavBus    = "javbus"
	MagnetSourceSukebei   = "sukebei"
	MagnetSourceFreeJavBT = "freejavbt"
)
