package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Subtitle is an online subtitle track of a movie.
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
	}
}
