package magnet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

type stubSource struct {
	name    string
	magnets []domain.Magnet
	err     error
	delay   time.Duration
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.magnets, s.err
}

func TestAggregatorDeduplicationAndMerging(t *testing.T) {
	hashCommon := "30291C52BB72D46AFFC1574EC01A4E16FC28A292"
	hashJavDBOnly := "1111111111111111111111111111111111111111"
	hashJavBusOnly := "2222222222222222222222222222222222222222"

	srcJavDB := &stubSource{name: domain.MagnetSourceJavDB, magnets: []domain.Magnet{
		{Hash: hashCommon, Name: "SSIS-001 Cleaned", Size: 2000, HD: true, CreatedAt: "2025-10-30", Sources: []string{domain.MagnetSourceJavDB}, Tags: []string{domain.MagnetTagHD}},
		{Hash: hashJavDBOnly, Name: "SSIS-001 JavDB Only", Size: 1000, HasSubtitle: true, Sources: []string{domain.MagnetSourceJavDB}, Tags: []string{domain.MagnetTagSubtitle}},
	}}
	srcJavBus := &stubSource{name: domain.MagnetSourceJavBus, magnets: []domain.Magnet{
		{Hash: hashCommon, Name: "SSIS-001 Raw", Size: 2500, HasSubtitle: true, CreatedAt: "2025-10-28", Sources: []string{domain.MagnetSourceJavBus}, Tags: []string{domain.MagnetTagSubtitle}},
		{Hash: hashJavBusOnly, Name: "SSIS-001 JavBus Only", Size: 5000, HD: true, Sources: []string{domain.MagnetSourceJavBus}, Tags: []string{domain.MagnetTagHD}},
	}}

	results, err := NewAggregator([]Source{srcJavBus, srcJavDB}, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 merged magnets, got %d", len(results))
	}

	common := results[0]
	if common.Hash != "30291c52bb72d46affc1574ec01a4e16fc28a292" {
		t.Fatalf("expected the merged magnet first with a lower-cased hash, got %+v", common)
	}
	if common.Name != "SSIS-001 Cleaned" || common.Size != 2500 || !common.HasSubtitle || !common.HD || common.CreatedAt != "2025-10-28" {
		t.Errorf("merge rules violated: %+v", common)
	}
	if len(common.Sources) != 2 || common.Sources[0] != domain.MagnetSourceJavDB || common.Sources[1] != domain.MagnetSourceJavBus {
		t.Errorf("expected sources [javdb javbus] regardless of source order, got %v", common.Sources)
	}
	if results[1].Hash != hashJavDBOnly || results[2].Hash != hashJavBusOnly {
		t.Errorf("expected subtitle before HD-only: %s, %s", results[1].Hash, results[2].Hash)
	}
}

func TestAggregatorMarksInferredTags(t *testing.T) {
	src := &stubSource{name: domain.MagnetSourceJavBus, magnets: []domain.Magnet{
		{Hash: "3333333333333333333333333333333333333333", Name: "SSIS-001-C 4K", Sources: []string{domain.MagnetSourceJavBus}},
		{Hash: "4444444444444444444444444444444444444444", Name: "SSIS-001", HD: true, Tags: []string{domain.MagnetTagHD}, Sources: []string{domain.MagnetSourceJavBus}},
	}}
	results, err := NewAggregator([]Source{src}, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatal(err)
	}
	inferred, verified := results[0], results[1]
	if !inferred.Inferred || inferred.HasSubtitle || !HasSubtitle(inferred) || !IsHD(inferred) {
		t.Errorf("name-derived tags must be marked inferred without touching site flags: %+v", inferred)
	}
	if verified.Inferred {
		t.Errorf("site-labelled magnet must not be marked inferred: %+v", verified)
	}
}

func TestAggregatorSingleSourceFailure(t *testing.T) {
	ok := &stubSource{name: domain.MagnetSourceJavDB, magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "Item 1"}}}
	broken := &stubSource{name: domain.MagnetSourceJavBus, err: errors.New("network timeout")}
	results, err := NewAggregator([]Source{ok, broken}, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil || len(results) != 1 {
		t.Fatalf("one healthy source must succeed: %v, %d results", err, len(results))
	}
}

func TestAggregatorSingleSourceTimeout(t *testing.T) {
	fast := &stubSource{name: domain.MagnetSourceJavDB, magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "Fast"}}}
	slow := &stubSource{name: domain.MagnetSourceJavBus, delay: time.Second, magnets: []domain.Magnet{{Hash: "2222222222222222222222222222222222222222", Name: "Slow"}}}
	started := time.Now()
	results, err := NewAggregator([]Source{fast, slow}, 50*time.Millisecond, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatalf("a timed-out source must not fail the query: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Fast" {
		t.Fatalf("expected only the fast source's magnet, got %+v", results)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("aggregator waited for the slow source past its timeout")
	}
}

func TestAggregatorAllSourcesFailed(t *testing.T) {
	sources := []Source{
		&stubSource{name: domain.MagnetSourceJavDB, err: errors.New("javdb down")},
		&stubSource{name: domain.MagnetSourceJavBus, err: errors.New("javbus blocked")},
	}
	_, err := NewAggregator(sources, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("expected KindUpstream when all sources fail, got %v", err)
	}
}

func TestAggregatorSkippedSourceDoesNotMaskFailure(t *testing.T) {
	sources := []Source{
		&stubSource{name: domain.MagnetSourceJavDB, err: errors.New("javdb down")},
		&stubSource{name: domain.MagnetSourceJavBus, err: ErrSkipped},
	}
	_, err := NewAggregator(sources, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("expected KindUpstream when the only active source fails, got %v", err)
	}
}

func TestAggregatorEmptyResultWithPartialFailureFails(t *testing.T) {
	sources := []Source{
		&stubSource{name: domain.MagnetSourceJavDB, err: errors.New("javdb down")},
		&stubSource{name: domain.MagnetSourceJavBus, magnets: nil},
	}
	_, err := NewAggregator(sources, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("expected KindUpstream when zero magnets found and a source failed, got %v", err)
	}
}

func TestAggregatorFindDetailedReportsPartialFailure(t *testing.T) {
	ok := &stubSource{name: domain.MagnetSourceJavDB, magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "Item 1"}}}
	broken := &stubSource{name: domain.MagnetSourceJavBus, err: errors.New("network timeout")}
	results, partial, err := NewAggregator([]Source{ok, broken}, time.Second, nil).FindDetailed(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil || len(results) != 1 || !partial {
		t.Fatalf("expected 1 result with partial=true and err=nil, got results=%d partial=%v err=%v", len(results), partial, err)
	}
}

func TestAggregatorAllSourcesSkipped(t *testing.T) {
	sources := []Source{
		&stubSource{name: domain.MagnetSourceJavDB, err: ErrSkipped},
		&stubSource{name: domain.MagnetSourceJavBus, err: ErrSkipped},
	}
	results, err := NewAggregator(sources, time.Second, nil).Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil || len(results) != 0 {
		t.Fatalf("expected 0 results and nil err when all skipped, got results=%d err=%v", len(results), err)
	}
}
