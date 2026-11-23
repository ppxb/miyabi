package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// StorageSource is one configured storage location (a local folder or a cloud
// drive). position keeps the order shown on the settings page.
type StorageSource struct {
	ent.Schema
}

func (StorageSource) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (StorageSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			NotEmpty().
			Unique().
			Immutable(),
		field.String("kind").
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.Bool("enabled").
			Default(true),
		field.JSON("config", map[string]string{}).
			Optional(),
		field.Int("position").
			Default(0),
	}
}
