package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ActorProfile is the project-canonical actor identity, decoupled from any single
// provider. Provider-scoped rows live in Actor and point here via the profile edge.
type ActorProfile struct {
	ent.Schema
}

func (ActorProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ActorProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("match_key").
			NotEmpty().
			Unique().
			Comment("Normalized name used to merge provider-scoped actors."),
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

func (ActorProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("sources", Actor.Type).
			Ref("profile"),
		edge.To("aliases", ActorAlias.Type),
		edge.To("images", ActorImage.Type),
	}
}
