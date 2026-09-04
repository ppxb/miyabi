package javdb

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type wireEnvelope struct {
	Success wireSuccess     `json:"success"`
	Action  string          `json:"action"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type wireSuccess bool

func (success *wireSuccess) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", "1":
		*success = true
	case "false", "0":
		*success = false
	default:
		return fmt.Errorf("decode success flag %q", data)
	}
	return nil
}

func decodeEnvelope(body []byte, destination any) error {
	var envelope wireEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode JavDB envelope: %w", err)
	}
	if !bool(envelope.Success) {
		return &APIError{Action: envelope.Action, Message: envelope.Message}
	}
	if destination == nil {
		return nil
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return fmt.Errorf("decode JavDB envelope: missing data")
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		return fmt.Errorf("decode JavDB data: %w", err)
	}
	return nil
}

type wireMoviesData struct {
	Movies []wireMovie `json:"movies"`
}

type wireMovieData struct {
	Movie wireMovie `json:"movie"`
}

type wireMovie struct {
	ID              string             `json:"id"`
	Number          string             `json:"number"`
	Title           string             `json:"title"`
	OriginTitle     string             `json:"origin_title"`
	ReleaseDate     string             `json:"release_date"`
	Duration        int                `json:"duration"`
	Score           float64            `json:"score"`
	ThumbURL        string             `json:"thumb_url"`
	CoverURL        string             `json:"cover_url"`
	PreviewImages   []wirePreviewImage `json:"preview_images"`
	PreviewVideoURL string             `json:"preview_video_url"`
	MagnetsCount    int                `json:"magnets_count"`
	HasSubtitle     bool               `json:"has_subtitle"`
	HasPreview      bool               `json:"has_preview"`
	Actors          []wireActor        `json:"actors"`
	Tags            []wireTag          `json:"tags"`
	SeriesID        string             `json:"series_id"`
	SeriesName      string             `json:"series_name"`
	MakerID         string             `json:"maker_id"`
	MakerName       string             `json:"maker_name"`
	DirectorID      string             `json:"director_id"`
	DirectorName    string             `json:"director_name"`
}

type wirePreviewImage struct {
	ThumbURL string `json:"thumb_url"`
	LargeURL string `json:"large_url"`
}

type wireActor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	NameZHT   string `json:"name_zht"`
	Gender    string `json:"gender"`
	AvatarURL string `json:"avatar_url"`
}

type wireTag struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	NameZHT    string `json:"name_zht"`
	CategoryID string `json:"category_id"`
}

type wireTagsData struct {
	Tags []wireTagCategory `json:"tags"`
}

type wireTagCategory struct {
	ID   string    `json:"category_id"`
	Name string    `json:"category"`
	Tags []wireTag `json:"tags"`
}
