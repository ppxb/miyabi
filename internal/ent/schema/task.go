package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Task struct {
	ent.Schema
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").
			NotEmpty(),
		field.Enum("status").
			Values("queued", "running", "done", "failed").
			Default("queued"),
		field.JSON("payload", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }),
		field.Int("progress").
			Range(0, 100).
			Default(0),
		field.String("error").
			Optional().
			Nillable(),
	}
}

func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
	}
}
