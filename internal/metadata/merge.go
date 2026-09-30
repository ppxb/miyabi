package metadata

import "github.com/ppxb/miyabi/internal/domain"

// merge only combines results confirmed at the same matching layer, in the
// configured priority order. Actor lists are selected as a whole: names alone
// cannot prove cross-provider identity.
func merge(results []domain.MovieMetadata) domain.MovieMetadata {
	var result domain.MovieMetadata
	result.Detail.FieldSources = make(map[string]string)
	images := make(map[string]bool)
	tags := make(map[string]bool)
	for _, item := range results {
		m := item.Detail
		if m.Code == "" {
			continue
		}
		provider := m.Sources[0].Provider
		out := &result.Detail
		out.Sources = append(out.Sources, m.Sources...)
		text := func(name string, dest *string, value string) {
			if *dest == "" && value != "" {
				*dest = value
				out.FieldSources[name] = provider
			}
		}
		text("code", &out.Code, m.Code)
		text("title", &out.Title, m.Title)
		text("summary", &out.Summary, m.Summary)
		text("release_date", &out.ReleaseDate, m.ReleaseDate)
		text("cover", &out.Cover, m.Cover)
		text("thumbnail", &out.Thumbnail, m.Thumbnail)
		text("preview_video", &out.PreviewVideo, m.PreviewVideo)
		if out.Zone == "" {
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
		if len(out.Actors) == 0 && len(m.Actors) > 0 {
			out.Actors = m.Actors
			out.FieldSources["actors"] = provider
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
		for _, tag := range m.Tags {
			if tag.Name != "" && !tags[tag.Name] {
				out.Tags = append(out.Tags, tag)
				tags[tag.Name] = true
			}
		}
		for _, image := range item.Images {
			key := image.Role + ":" + image.URL
			if image.URL == "" || images[key] {
				continue
			}
			images[key] = true
			result.Images = append(result.Images, image)
			if image.Role == "preview" {
				out.PreviewImages = append(out.PreviewImages, domain.PreviewImage{Original: image.URL, Thumbnail: image.URL})
			}
		}
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0 || result.Detail.PreviewVideo != ""
	return result
}
