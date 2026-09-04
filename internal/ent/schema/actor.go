package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Actor struct {
	ent.Schema
}

func (Actor) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Actor) Fields() []ent.Field {
	return []ent.Field{
		field.String("javdb_id").
			NotEmpty().
			Unique(),
		field.String("name").
			NotEmpty(),
		field.String("name_zht").
			Optional().
			Nillable(),
		field.Enum("gender").
			Values("female", "male", "unknown").
			Default("unknown"),
		field.String("avatar").
			Optional().
			Nillable(),
	}
}

func (Actor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movies", Movie.Type).
			Ref("actors"),
	}
}
