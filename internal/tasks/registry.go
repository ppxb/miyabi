package tasks

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/ppxb/miyabi/internal/ent"
)

// Job is internal execution input. API responses never expose raw payloads.
type Job struct {
	ID      int             `json:"id"`
	Type    Kind            `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type HandleFunc func(ctx context.Context, job Job) error
type FinishedFunc func(ctx context.Context, tx *ent.Tx, job Job, result error) (Change, error)

// Handler executes jobs of one Kind. Finished is optional and runs inside the
// completion transaction; its revisions are published only after commit.
type Handler struct {
	Kind     Kind
	Handle   HandleFunc
	Finished FinishedFunc
}

// NewHandler pairs an execution function with an optional completion callback.
func NewHandler(kind Kind, handle HandleFunc, finished FinishedFunc) Handler {
	return Handler{Kind: kind, Handle: handle, Finished: finished}
}

// Registry maps task kinds to their handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[Kind]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[Kind]Handler)}
}

// Register installs a handler, replacing any previous one for the same Kind.
func (r *Registry) Register(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Kind] = h
}

func (r *Registry) Get(k Kind) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[k]
	return h, ok
}

// Kinds returns registered kinds in sorted order.
func (r *Registry) Kinds() []Kind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]Kind, 0, len(r.handlers))
	for k := range r.handlers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}
