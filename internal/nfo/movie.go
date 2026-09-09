package nfo

import (
	"encoding/xml"
	"fmt"
	"net/url"
)

// Kodi reads the standard fields. Source IDs and category/gender attributes
// keep a Miyabi rescan lossless without requesting JavDB again.
type Movie struct {
	XMLName   xml.Name   `xml:"movie" json:"-"`
	Title     string     `xml:"title" json:"title"`
	Code      string     `xml:"num" json:"code"`
	IDs       []UniqueID `xml:"uniqueid" json:"ids"`
	Premiered string     `xml:"premiered,omitempty" json:"premiered"`
	Runtime   int        `xml:"runtime,omitempty" json:"runtime"`
	Rating    float64    `xml:"rating,omitempty" json:"rating"`
	Director  Entity     `xml:"director" json:"director"`
	Studio    Entity     `xml:"studio" json:"studio"`
	Set       Series     `xml:"set" json:"set"`
	Actors    []Actor    `xml:"actor" json:"actors"`
	Tags      []Tag      `xml:"tag" json:"tags"`
	Genres    []string   `xml:"genre" json:"genres"`
	Thumbs    []Thumb    `xml:"thumb" json:"thumbs"`
	Fanart    string     `xml:"fanart>thumb,omitempty" json:"fanart"`
}

type UniqueID struct {
	Type    string `xml:"type,attr" json:"type"`
	Default bool   `xml:"default,attr,omitempty" json:"default"`
	Value   string `xml:",chardata" json:"value"`
}

type Entity struct {
	ID   string `xml:"javdbid,attr,omitempty" json:"id"`
	Name string `xml:",chardata" json:"name"`
}

type Series struct {
	ID   string `xml:"javdbid,attr,omitempty" json:"id"`
	Name string `xml:"name,omitempty" json:"name"`
}

type Actor struct {
	ID      string `xml:"javdbid,omitempty" json:"id"`
	Name    string `xml:"name" json:"name"`
	NameZHT string `xml:"name_zht,omitempty" json:"name_zht"`
	Gender  string `xml:"gender,omitempty" json:"gender"`
	Thumb   string `xml:"thumb,omitempty" json:"thumb"`
}

type Tag struct {
	ID         string `xml:"javdbid,attr,omitempty" json:"id"`
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
