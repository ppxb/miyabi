package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/ppxb/miyabi/internal/domain"
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
		field.String("name_key").
			Default("").
			Comment("sha1 of the normalized name, derived by a hook; indexed for O(log n) avatar lookups."),
	}
}

// Hooks derive name_key in the same write as name, including bulk upserts, so
// no caller maintains it by hand.
func (Actor) Hooks() []ent.Hook {
	return []ent.Hook{func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if name, ok := m.Field("name"); ok {
				if err := m.SetField("name_key", domain.ActorNameKey(name.(string))); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}}
}

func (Actor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movies", Movie.Type).
			Ref("actors"),
		edge.To("profile", ActorProfile.Type).
			Unique(),
	}
}

func (Actor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "source_id").Unique(),
		index.Fields("name_key"),
		index.Edges("profile"),
	}
}
