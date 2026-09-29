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
		field.String("target_id").
			NotEmpty(),
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
		field.Bool("auto_download").
			Default(true),
		field.Enum("status").
			Values("waiting", "added", "stale", "active", "paused").
			Default("waiting"),
		field.String("cursor").
			Default(""),
		field.String("hash").
			Default(""),
		field.Int("task_id").
			Optional().
			Nillable(),
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
