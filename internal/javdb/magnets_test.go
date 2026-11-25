package javdb

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestMagnetsFromWire(t *testing.T) {
	got := magnetsFromWire([]wireMagnet{
		{Hash: "ABCDEF0123456789ABCDEF0123456789ABCDEF01", Name: "sub", SizeMiB: 1024, CNSub: true, FilesCount: 1, CreatedAt: "2026-08-01"},
		{Hash: "", Name: "ignored"},
		{Hash: "0000000000000000000000000000000000000002", Name: "hd", SizeMiB: 16384, HD: true, FilesCount: 2},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Hash != "abcdef0123456789abcdef0123456789abcdef01" || got[0].Size != int64(1024)<<20 ||
		!got[0].HasSubtitle || len(got[0].Sources) != 1 || got[0].Sources[0] != domain.MagnetSourceJavDB {
		t.Fatalf("first=%+v", got[0])
	}
	if !got[1].HD || got[1].Size != int64(16384)<<20 {
		t.Fatalf("second=%+v", got[1])
	}
}
