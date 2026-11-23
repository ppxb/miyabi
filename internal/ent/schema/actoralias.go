package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ActorAlias is an alternative spelling (translation, old stage name, spacing
// variant) that resolves to one canonical ActorProfile.
type ActorAlias struct {
	ent.Schema
}

func (ActorAlias) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ActorAlias) Fields() []ent.Field {
	return []ent.Field{
		field.String("alias").NotEmpty(),
		field.String("normalized").NotEmpty(),
	}
}

func (ActorAlias) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("normalized"),
	}
}

func (ActorAlias) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("actor", ActorProfile.Type).
			Ref("aliases").
			Unique(),
	}
}
