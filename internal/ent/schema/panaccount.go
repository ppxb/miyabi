package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// PanAccount stores one 115 account's tokens and its mounted library directory.
// An empty directory_id means the account has no mount.
type PanAccount struct {
	ent.Schema
}

func (PanAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PanAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("account_id").
			NotEmpty().
			Unique().
			Immutable(),
		field.String("access_token").
			Default("").
			Sensitive(),
		field.String("refresh_token").
			Default("").
			Sensitive(),
		field.Time("expires_at").
			Optional(),
		field.String("directory_id").
			Default(""),
		field.String("directory_name").
			Default(""),
		field.String("directory_path").
			Default(""),
	}
}
