package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type File struct {
	ent.Schema
}

func (File) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (File) Fields() []ent.Field {
	return []ent.Field{
		field.String("file_id").
			NotEmpty().
			Unique(),
		field.String("pick_code").
			Default(""),
		field.String("sha1").
			Default(""),
		field.String("name").
			NotEmpty(),
		field.Int64("size").
			NonNegative(),
		field.String("parent_id").
			Default(""),
	}
}

func (File) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movie", Movie.Type).
			Ref("files").
			Unique(),
	}
}
