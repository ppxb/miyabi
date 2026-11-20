package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ViewedMovie struct {
	ent.Schema
}

func (ViewedMovie) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").NotEmpty().Unique(),
		field.Time("viewed_at").Default(time.Now),
	}
}

func (ViewedMovie) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("viewed_at", "id"),
	}
}
