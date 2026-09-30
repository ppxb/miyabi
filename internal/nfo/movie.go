package nfo

import (
	"encoding/xml"
	"fmt"
	"github.com/ppxb/miyabi/internal/domain"
	"net/url"
)

// Kodi reads the standard fields. Source IDs and category/gender attributes
// keep a Miyabi rescan lossless without requesting metadata sources again.
type Movie struct {
	SelectedImage domain.ImageCandidate   `xml:"-" json:"selected_image,omitempty"`
	Images        []domain.ImageCandidate `xml:"-" json:"images,omitempty"`
	Summary       string                  `xml:"plot,omitempty" json:"summary,omitempty"`
	RatingSource  string                  `xml:"rating_source,omitempty" json:"rating_source,omitempty"`
	RatingMax     float64                 `xml:"rating_max,omitempty" json:"rating_max,omitempty"`
	FieldSources  map[string]string       `xml:"-" json:"field_sources,omitempty"`
	Previews      []domain.PreviewImage   `xml:"-" json:"previews,omitempty"`
	PreviewVideo  string                  `xml:"trailer,omitempty" json:"preview_video,omitempty"`
	XMLName       xml.Name                `xml:"movie" json:"-"`
	Title         string                  `xml:"title" json:"title"`
	Code          string                  `xml:"num" json:"code"`
	IDs           []UniqueID              `xml:"uniqueid" json:"ids"`
	Premiered     string                  `xml:"premiered,omitempty" json:"premiered"`
	Runtime       int                     `xml:"runtime,omitempty" json:"runtime"`
	Rating        float64                 `xml:"rating,omitempty" json:"rating"`
	Director      Entity                  `xml:"director" json:"director"`
	Studio        Entity                  `xml:"studio" json:"studio"`
	Set           Series                  `xml:"set" json:"set"`
	Actors        []Actor                 `xml:"actor" json:"actors"`
	Tags          []Tag                   `xml:"tag" json:"tags"`
	Genres        []string                `xml:"genre" json:"genres"`
	Thumbs        []Thumb                 `xml:"thumb" json:"thumbs"`
	Fanart        string                  `xml:"fanart>thumb,omitempty" json:"fanart"`
}

type UniqueID struct {
	Type    string `xml:"type,attr" json:"type"`
	Default bool   `xml:"default,attr,omitempty" json:"default"`
	Value   string `xml:",chardata" json:"value"`
}

type Entity struct {
	Provider string `xml:"provider,attr,omitempty" json:"provider,omitempty"`
	ID       string `xml:"sourceid,attr,omitempty" json:"id"`
	Name     string `xml:",chardata" json:"name"`
}

type Series struct {
	Provider string `xml:"provider,attr,omitempty" json:"provider,omitempty"`
	ID       string `xml:"sourceid,attr,omitempty" json:"id"`
	Name     string `xml:"name,omitempty" json:"name"`
}

type Actor struct {
	Provider string `xml:"provider,attr,omitempty" json:"provider,omitempty"`
	ID       string `xml:"sourceid,omitempty" json:"id"`
	Name     string `xml:"name" json:"name"`
	NameZHT  string `xml:"name_zht,omitempty" json:"name_zht"`
	Gender   string `xml:"gender,omitempty" json:"gender"`
	// Actor images are uploaded by Miyabi after decoding, not fetched by Emby.
	Thumb string `xml:"miyabi_avatar,omitempty" json:"thumb"`
}

// Read ordinary NFO thumbnails as well as Miyabi's preserved source URL.
func (actor *Actor) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	type actorXML Actor
	var value struct {
		actorXML
		LegacyThumb string `xml:"thumb"`
	}
	if err := decoder.DecodeElement(&value, &start); err != nil {
		return err
	}
	*actor = Actor(value.actorXML)
	if actor.Thumb == "" {
		actor.Thumb = value.LegacyThumb
	}
	return nil
}

type Tag struct {
	Provider   string `xml:"provider,attr,omitempty" json:"provider,omitempty"`
	ID         string `xml:"sourceid,attr,omitempty" json:"id"`
	CategoryID string `xml:"category,attr,omitempty" json:"category_id"`
	NameZHT    string `xml:"name_zht,attr,omitempty" json:"name_zht"`
	Name       string `xml:",chardata" json:"name"`
}

type Thumb struct {
	Aspect string `xml:"aspect,attr" json:"aspect"`
	Path   string `xml:",chardata" json:"path"`
}

// FileStem encodes a catalogue number as one filename component. Metadata keeps
// the full number; characters such as slashes must not become directory paths.
func FileStem(code string) string {
	switch code {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	}
	return url.QueryEscape(code)
}

func (movie Movie) JavDBID() string {
	for _, id := range movie.IDs {
		if id.Type == "javdb" {
			return id.Value
		}
	}
	return ""
}

func (movie Movie) Poster() string {
	for _, thumb := range movie.Thumbs {
		if thumb.Aspect == "poster" {
			return thumb.Path
		}
	}
	return ""
}

func Decode(body []byte) (Movie, error) {
	var movie Movie
	if err := xml.Unmarshal(body, &movie); err != nil {
		return Movie{}, fmt.Errorf("decode movie NFO: %w", err)
	}
	if movie.Code == "" {
		for _, id := range movie.IDs {
			if id.Type == "code" {
				movie.Code = id.Value
			}
		}
	}
	return movie, nil
}

func Encode(movie Movie) ([]byte, error) {
	body, err := xml.MarshalIndent(movie, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode movie NFO: %w", err)
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}
