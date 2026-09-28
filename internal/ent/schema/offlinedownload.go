package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OfflineDownload records a remote 115 download independently of executable tasks.
type OfflineDownload struct{ ent.Schema }

func (OfflineDownload) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (OfflineDownload) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").Default(""),
		field.String("javdb_id").Default(""),
		field.String("hash").NotEmpty(),
		field.String("info_hash").Default(""),
		field.String("account_id").Default(""),
		field.String("directory_id").Default(""),
		field.Enum("status").Values("running", "done", "failed").Default("running"),
		field.Int("progress").Range(0, 100).Default(0),
		field.String("error").Optional().Nillable(),
		field.String("file_id").Default(""),
		field.JSON("file_ids", []string{}).Default([]string{}),
		field.Int("scan_task_id").Default(0),
		field.Bool("awaiting_location").Default(false),
	}
}

func (OfflineDownload) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "status", "hash"),
		index.Fields("account_id", "directory_id", "hash", "id"),
		index.Fields("account_id", "javdb_id", "hash", "id"),
	}
}
