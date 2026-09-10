package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		field.String("account_id").
			Default(""),
		field.String("root_id").
			Default(""),
		field.String("path").
			Default(""),
		field.String("scan_id").
			Default(""),
		field.Int("movie_id").
			StorageKey("movie_files").
			Optional().
			Nillable(),
	}
}

func (File) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movie", Movie.Type).
			Ref("files").
			Field("movie_id").
			Unique(),
	}
}

func (File) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "root_id", "scan_id"),
		index.Fields("movie_id", "account_id", "root_id"),
	}
}
