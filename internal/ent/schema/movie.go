package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
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
		field.String("canonical_code").
			Default("").
			Comment("Candidate grouping key; equivalence must still be checked against code."),
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
		field.JSON("metadata_snapshot", &domain.MetadataSnapshot{}).Optional(),
		field.Enum("scrape_status").
			Values("pending", "done", "failed").
			Default("pending"),
	}
}

func (Movie) Indexes() []ent.Index {
	return []ent.Index{index.Fields("canonical_code")}
}

// Keep the lookup key in the same write as code, including bulk creates and
// metadata updates, without making each caller maintain a derived field.
func (Movie) Hooks() []ent.Hook {
	return []ent.Hook{func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if code, ok := m.Field("code"); ok {
				if err := m.SetField("canonical_code", codeid.MatchKey(code.(string))); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}}
}

func (Movie) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("actors", Actor.Type),
		edge.To("tags", Tag.Type),
		edge.To("files", File.Type),
		edge.To("subtitles", Subtitle.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
