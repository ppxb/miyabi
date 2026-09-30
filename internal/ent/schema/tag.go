package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Tag struct {
	ent.Schema
}

func (Tag) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").NotEmpty(),
		field.String("source_id").NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.String("name_zht").
			Optional().
			Nillable(),
		field.String("category_id").Default(""),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movies", Movie.Type).
			Ref("tags"),
	}
}

func (Tag) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider", "source_id").Unique()}
}
