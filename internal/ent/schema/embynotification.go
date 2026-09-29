package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EmbyNotification is an outstanding directory update, not an Emby library item.
type EmbyNotification struct{ ent.Schema }

func (EmbyNotification) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (EmbyNotification) Fields() []ent.Field {
	return []ent.Field{
		field.String("path").NotEmpty().Unique(),
		field.Int("revision").Default(1),
		field.Int("attempts").Default(0),
		field.Time("next_attempt_at").Default(time.Now),
		field.String("last_error").Default(""),
	}
}

func (EmbyNotification) Indexes() []ent.Index {
	return []ent.Index{index.Fields("next_attempt_at")}
}
