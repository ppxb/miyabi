package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Actor struct {
	ent.Schema
}

func (Actor) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Actor) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").NotEmpty(),
		field.String("source_id").NotEmpty(),
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

func (Actor) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider", "source_id").Unique()}
}
