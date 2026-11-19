package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ActorImage is one persisted image for a canonical actor (e.g. a gfriends
// portrait), ordered by position.
type ActorImage struct {
	ent.Schema
}

func (ActorImage) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ActorImage) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").Default(""),
		field.String("source_id").Default(""),
		field.String("url").NotEmpty(),
		field.String("role").Default(""),
		field.Int("position").Default(0),
	}
}

func (ActorImage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("actor", ActorProfile.Type).
			Ref("images").
			Unique(),
	}
}
