package magnet

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
)

// Source provides magnets with valid lowercase info hashes and Sources containing
// its Name. Site flags and their display tags are populated before aggregation.
type Source interface {
	Name() string
	Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error)
}
