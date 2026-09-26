package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Subtitle is a subtitle track of a movie. Tracks found beside 115 videos
// keep their file_id and pick_code; online tracks keep their source_url.
// storage_path is the file exported beside the movie's .strm, empty until exported.
type Subtitle struct {
	ent.Schema
}

func (Subtitle) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Subtitle) Fields() []ent.Field {
	return []ent.Field{
		field.Int("movie_id").
			Positive(),
		field.String("file_id").
			Default(""),
		field.String("pick_code").
			Default(""),
		field.String("name").
			NotEmpty(),
		field.String("language").
			Default("zh-CN"),
		field.String("format").
			Default("srt"),
		field.String("version_tag").
			Default("standard"),
		field.String("source").
			Default("local"),
		field.String("source_url").
			Default(""),
		field.String("storage_path").
			Default(""),
	}
}

func (Subtitle) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movie", Movie.Type).
			Ref("subtitles").
			Field("movie_id").
			Unique().
			Required(),
	}
}

func (Subtitle) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("movie_id"),
		index.Fields("file_id"),
	}
}
