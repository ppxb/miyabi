package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Movie struct {
	ent.Schema
}

func (Movie) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Movie) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			NotEmpty().
			Unique(),
		field.String("javdb_id").
			Optional().
			Nillable().
			Unique(),
		field.String("title").
			Default(""),
		field.Time("release_date").
			Optional().
			Nillable(),
		field.Int("duration").
			Optional().
			Nillable(),
		field.String("director_id").
			Optional().
			Nillable(),
		field.String("director_name").
			Optional().
			Nillable(),
		field.String("maker_id").
			Optional().
			Nillable(),
		field.String("maker_name").
			Optional().
			Nillable(),
		field.String("series_id").
			Optional().
			Nillable(),
		field.String("series_name").
			Optional().
			Nillable(),
		field.Float("rating").
			Optional().
			Nillable(),
		field.String("cover").
			Optional().
			Nillable(),
		field.String("poster").
			Optional().
			Nillable(),
		field.JSON("fanarts", []string{}).
			Default(func() []string { return []string{} }),
		field.Enum("scrape_status").
			Values("pending", "done", "failed").
			Default("pending"),
	}
}

func (Movie) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("actors", Actor.Type),
		edge.To("tags", Tag.Type),
		edge.To("files", File.Type),
	}
}
