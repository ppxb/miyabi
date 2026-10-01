package schema

import (
	"encoding/json"

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
		field.JSON("payload", json.RawMessage{}).
			Default(func() json.RawMessage { return json.RawMessage(`{}`) }),
		field.Int("progress").
			Range(0, 100).
			Default(0),
		field.Int("retry_count").NonNegative().Default(0),
		field.Time("retry_at").Optional().Nillable(),
		// Jobs sharing a resource run in order, including delayed retries.
		field.String("resource_key").Default(""),
		field.String("error").
			Optional().
			Nillable(),
	}
}

func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("type", "status"),
		index.Fields("type", "status", "retry_at"),
		index.Fields("resource_key", "status"),
		// Keep history across statuses ordered by SQLite's implicit row ID.
		index.Fields("type"),
	}
}
