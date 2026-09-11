package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type WatchHistory struct {
	ent.Schema
}

func (WatchHistory) Fields() []ent.Field {
	return []ent.Field{
		field.String("account_id").NotEmpty(),
		field.String("root_id").NotEmpty(),
		field.Int("movie_id").Positive(),
		field.Time("watched_at").Default(time.Now),
		field.String("session_id").NotEmpty(),
		field.String("file_id").Default(""),
		field.Float("position").Default(0),
		field.Float("duration").Default(0),
		field.Int("progress_version").NonNegative().Default(0),
	}
}

func (WatchHistory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movie", Movie.Type).
			Ref("watch_history").
			Field("movie_id").
			Unique().Required(),
	}
}

func (WatchHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "root_id", "movie_id").Unique(),
		index.Fields("account_id", "root_id", "watched_at", "id"),
		index.Fields("movie_id"),
	}
}
