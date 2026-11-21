package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WatchHistory stores playback progress per library movie.
type WatchHistory struct {
	ent.Schema
}

func (WatchHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int("movie_id").Positive().Unique(),
		field.String("file_id").Default(""),
		field.Float("position").Default(0),
		field.Float("duration").Default(0),
		// settings is a single extensible JSON blob for all per-movie playback
		// preferences (speed, rendition, and later subtitles/audio tracks ...), so
		// adding a new preference needs no schema change.
		field.JSON("settings", json.RawMessage{}).Optional(),
		field.Time("watched_at").Default(time.Now),
	}
}

func (WatchHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("watched_at", "id"),
	}
}
