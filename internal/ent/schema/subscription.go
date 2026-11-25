package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Subscription tracks wanted movies or actors to automate library intake.
type Subscription struct {
	ent.Schema
}

func (Subscription) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Subscription) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("kind").
			Values("movie", "actor").
			Default("movie"),
		// target_id is the library actor key for actors and the code for movies.
		field.String("target_id").
			NotEmpty(),
		// The ids an actor's new works are listed by: FANZA first, JavDB as fallback.
		field.String("fanza_id").
			Default(""),
		field.String("javdb_id").
			Default(""),
		field.String("code").
			Default(""),
		field.String("title").
			Default(""),
		field.String("cover").
			Default(""),
		field.String("release_date").
			Default(""),
		field.Int("origin_id").
			Optional().
			Nillable(),
		field.Enum("status").
			Values("waiting", "added", "stale", "active", "paused").
			Default("waiting"),
		field.String("cursor").
			Default(""),
		field.Time("next_check_at").
			Optional().
			Nillable(),
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		field.Int("checks").
			NonNegative().
			Default(0),
		field.String("error").
			Optional().
			Nillable(),
	}
}

func (Subscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "target_id").Unique(),
		index.Fields("status", "next_check_at"),
	}
}
