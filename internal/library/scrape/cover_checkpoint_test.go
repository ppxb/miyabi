package scrape

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestArtworkCheckpointFailureReleasesLockAndCanRetry(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 12, 8))); err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"decode", "checkpoint", "cancelled lock wait"} {
		t.Run(failure, func(t *testing.T) {
			ctx := t.Context()
			store, err := database.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			images, err := mediaimage.NewCache(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			service := &Service{db: store.Client, images: images}
			job := store.Client.Task.Create().SetType("cover").SaveX(ctx)
			failWrite := failure == "checkpoint"
			writeError := errors.New("fixture checkpoint failure")
			store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					if failWrite {
						return nil, writeError
					}
					return next.Mutate(ctx, mutation)
				})
			})
			body := encoded.Bytes()
			if failure == "decode" {
				body = []byte("invalid image")
			}
			if failure == "cancelled lock wait" {
				if err := images.LockArtwork(ctx); err != nil {
					t.Fatal(err)
				}
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			input := CoverPayload{}
			err = service.checkpointArtwork(ctx, job.ID, &input, body)
			if failure == "cancelled lock wait" {
				images.UnlockArtwork()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled lock wait = %v", err)
				}
			}
			if err == nil || failWrite && !errors.Is(err, writeError) {
				t.Fatalf("checkpoint failure = %v", err)
			}
			if !images.TryLockArtwork() {
				t.Fatal("failed checkpoint leaked the artwork lock")
			}
			images.UnlockArtwork()
			saved := store.Client.Task.GetX(t.Context(), job.ID)
			if !bytes.Equal(saved.Payload, job.Payload) {
				t.Fatal("failed checkpoint published partial artwork references")
			}
			failWrite = false
			input = CoverPayload{}
			if err := service.checkpointArtwork(t.Context(), job.ID, &input, encoded.Bytes()); err != nil {
				t.Fatalf("checkpoint retry: %v", err)
			}
			saved = store.Client.Task.GetX(t.Context(), job.ID)
			checkpoint, err := tasks.DecodePayload[CoverPayload](saved.Payload)
			if err != nil || checkpoint.Artwork == nil || *checkpoint.Artwork != *input.Artwork || checkpoint.Completed {
				t.Fatalf("retry checkpoint = %+v, %v", checkpoint, err)
			}
		})
	}
}
