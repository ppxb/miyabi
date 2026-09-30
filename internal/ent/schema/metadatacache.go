package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/ppxb/miyabi/internal/domain"
)

type MetadataCache struct{ ent.Schema }

func (MetadataCache) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").NotEmpty(),
		field.String("code").NotEmpty(),
		field.JSON("result", &domain.MovieMetadata{}).Optional(),
		field.Time("expires_at"),
	}
}

func (MetadataCache) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider", "code").Unique()}
}
