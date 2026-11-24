package metadata

import (
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

// merge combines confirmed results for the same film. No source is privileged:
// single-value fields fill from the first source (in the caller's source order)
// that has a value, and artwork keeps that same order. Actors and tags are
// unioned across providers (deduplicated by name) so every source contributes.
func merge(results []domain.MovieMetadata) domain.MovieMetadata {
	var result domain.MovieMetadata
	result.Detail.FieldSources = make(map[string]string)
	images := make(map[string]bool)
	actors := make(map[string]bool)
	tags := make(map[string]bool)
	javdbPlot, javdbPlotSource := "", ""
	for _, item := range results {
		for _, image := range item.Images {
			key := image.Role + ":" + image.URL
			if image.URL == "" || images[key] {
				continue
			}
			images[key] = true
			result.Images = append(result.Images, image)
			if image.Role == "preview" {
				result.Detail.PreviewImages = append(result.Detail.PreviewImages, domain.PreviewImage{Original: image.URL, Thumbnail: image.URL})
			}
		}
	}
	// Keep the caller's source order; no provider is privileged.
	results = slices.Clone(results)
	for _, item := range results {
		m := item.Detail
		if m.Code == "" {
			continue
		}
		provider := m.Sources[0].Provider
		out := &result.Detail
		out.Sources = append(out.Sources, m.Sources...)
		// Related-work panels come from whichever source renders them; JavDB first
		// when present, otherwise a supplementary source such as freejavbt.
		if len(out.ActorMovies) == 0 && len(m.ActorMovies) > 0 {
			out.ActorMovies = m.ActorMovies
		}
		if len(out.RelatedMovies) == 0 && len(m.RelatedMovies) > 0 {
			out.RelatedMovies = m.RelatedMovies
		}
		text := func(name string, dest *string, value string) {
			if *dest == "" && value != "" {
				*dest = value
				out.FieldSources[name] = provider
			}
		}
		text("code", &out.Code, m.Code)
		// Title prefers the Japanese original (OriginTitle) over a localized
		// title, matching the configured "prefer official original" policy.
		if out.Title == "" {
			switch {
			case m.OriginTitle != "":
				out.Title, out.FieldSources["title"] = m.OriginTitle, provider
			case m.Title != "":
				out.Title, out.FieldSources["title"] = m.Title, provider
			}
		}
		text("origin_title", &out.OriginTitle, m.OriginTitle)
		text("release_date", &out.ReleaseDate, m.ReleaseDate)
		text("cover", &out.Cover, m.Cover)
		text("thumbnail", &out.Thumbnail, m.Thumbnail)
		text("preview_video", &out.PreviewVideo, m.PreviewVideo)
		// Synopsis prefers any official source; JavDB is only the fallback.
		if m.Plot != "" {
			if provider == "javdb" {
				if javdbPlot == "" {
					javdbPlot, javdbPlotSource = m.Plot, provider
				}
			} else if out.Plot == "" {
				out.Plot = m.Plot
				out.FieldSources["plot"] = provider
			}
		}
		if out.Zone == "" || out.Zone == domain.ZoneUnknown {
			out.Zone = m.Zone
		}
		if out.Duration == 0 && m.Duration > 0 {
			out.Duration = m.Duration
			out.FieldSources["duration"] = provider
		}
		if out.Rating == 0 && m.Rating > 0 {
			out.Rating = m.Rating
			out.RatingMax = m.RatingMax
			out.RatingSource = provider
		}
		for _, person := range m.Actors {
			key := "name:" + strings.ToLower(strings.TrimSpace(person.Name))
			if strings.TrimSpace(person.Name) == "" {
				key = person.Provider + ":" + person.ID
			}
			if actors[key] {
				continue
			}
			actors[key] = true
			out.Actors = append(out.Actors, person)
			if _, ok := out.FieldSources["actors"]; !ok {
				out.FieldSources["actors"] = provider
			}
		}
		if out.Maker == nil && m.Maker != nil {
			out.Maker = m.Maker
			out.FieldSources["maker"] = provider
		}
		if out.Director == nil && m.Director != nil {
			out.Director = m.Director
			out.FieldSources["director"] = provider
		}
		if out.Series == nil && m.Series != nil {
			out.Series = m.Series
			out.FieldSources["series"] = provider
		}
		for _, label := range m.Tags {
			key := "name:" + strings.ToLower(strings.TrimSpace(label.Name))
			if strings.TrimSpace(label.Name) == "" {
				key = label.Provider + ":" + label.ID
			}
			if tags[key] {
				continue
			}
			tags[key] = true
			out.Tags = append(out.Tags, label)
			if _, ok := out.FieldSources["tags"]; !ok {
				out.FieldSources["tags"] = provider
			}
		}
	}
	if result.Detail.Plot == "" && javdbPlot != "" {
		result.Detail.Plot = javdbPlot
		result.Detail.FieldSources["plot"] = javdbPlotSource
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0 || result.Detail.PreviewVideo != ""
	return result
}
