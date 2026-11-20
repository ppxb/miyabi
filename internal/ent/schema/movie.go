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
	"github.com/ppxb/miyabi/internal/nfo"
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
		field.String("manual_code").Default(""),
		field.String("canonical_code").
			Default("").
			Comment("Candidate grouping key; equivalence must still be checked against code."),
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
		field.Float("rating10").
			Default(0).
			Comment("Normalized 10-point rating derived from rating and metadata rating_max."),
		field.String("cover").
			Optional().
			Nillable(),
		field.String("poster").
			Optional().
			Nillable(),
		field.JSON("fanarts", []string{}).
			Default(func() []string { return []string{} }),
		field.JSON("metadata", &nfo.Movie{}).Optional(),
		field.JSON("metadata_snapshot", &domain.MetadataSnapshot{}).Optional(),
		field.Enum("scrape_status").
			Values("pending", "done", "failed").
			Default("pending"),
	}
}

func (Movie) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("canonical_code"),
		index.Fields("created_at", "id"),
		index.Fields("rating10", "id"),
	}
}

// Keep the lookup key and the normalized rating in the same write as their
// source fields, including bulk creates and metadata updates, without making
// each caller maintain a derived field.
func (Movie) Hooks() []ent.Hook {
	return []ent.Hook{func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if code, ok := m.Field("code"); ok {
				if err := m.SetField("canonical_code", codeid.MatchKey(code.(string))); err != nil {
					return nil, err
				}
			}
			if err := deriveRating10(m); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, m)
		})
	}}
}

// deriveRating10 keeps the stored normalized rating (rating10) in sync whenever
// the raw rating or the metadata (which carries rating_max) is written. When the
// raw rating is written without metadata it falls back to the 5-point default,
// matching how an absent metadata snapshot is normalized elsewhere.
func deriveRating10(m ent.Mutation) error {
	if meta, ok := m.Field("metadata"); ok {
		var max, rating float64
		doc, _ := meta.(*nfo.Movie)
		if doc != nil {
			max = doc.RatingMax
		}
		if raw, ok := m.Field("rating"); ok {
			rating, _ = raw.(float64)
		} else if doc != nil {
			rating = doc.Rating
		}
		return m.SetField("rating10", domain.NormalizeRating10(rating, max))
	}
	if raw, ok := m.Field("rating"); ok {
		rating, _ := raw.(float64)
		return m.SetField("rating10", domain.NormalizeRating10(rating, 0))
	}
	return nil
}

func (Movie) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("actors", Actor.Type),
		edge.To("tags", Tag.Type),
		edge.To("files", File.Type),
		edge.To("subtitles", Subtitle.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
