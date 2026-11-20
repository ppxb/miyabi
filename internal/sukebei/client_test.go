package sukebei

import (
	"context"
	"io"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
)

type fakeHTTP struct {
	body   string
	status int
	calls  int
}

func (f *fakeHTTP) Do(*http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

func (*fakeHTTP) CloseIdleConnections() {}

func TestFindMatchesCode(t *testing.T) {
	fake := &fakeHTTP{body: sampleFeed, status: 200}
	client := NewForTest(fake)
	defer client.Close()

	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: "ssis-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(magnets) != 1 {
		t.Fatalf("magnets = %d, want 1", len(magnets))
	}
	if magnets[0].Hash != "abcdef0123456789abcdef0123456789abcdef01" || magnets[0].Sources[0] != domain.MagnetSourceSukebei {
		t.Fatalf("magnet = %+v", magnets[0])
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
}

func TestFindSkipsEmptyCode(t *testing.T) {
	fake := &fakeHTTP{body: sampleFeed, status: 200}
	client := NewForTest(fake)
	defer client.Close()

	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: ""})
	if err != nil || len(magnets) != 0 || fake.calls != 0 {
		t.Fatalf("magnets = %v, err = %v, calls = %d", magnets, err, fake.calls)
	}
}

func TestFindRejectsChallenge(t *testing.T) {
	fake := &fakeHTTP{body: "<html>Just a moment...</html>", status: 200}
	client := NewForTest(fake)
	defer client.Close()

	if _, err := client.Find(context.Background(), domain.MovieRef{Code: "SSIS-001"}); err == nil {
		t.Fatal("expected a challenge error, got nil")
	}
}

const fc2Feed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://sukebei.nyaa.si/xmlns/nyaa"><channel>
<item><title>FC2-PPV-1234567 [1080p]</title><nyaa:infoHash>2222222222222222222222222222222222222222</nyaa:infoHash><nyaa:size>2 GiB</nyaa:size></item>
</channel></rss>`

func TestFindMatchesFC2Code(t *testing.T) {
	fake := &fakeHTTP{body: fc2Feed, status: 200}
	client := NewForTest(fake)
	defer client.Close()

	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: "FC2-PPV-1234567"})
	if err != nil || len(magnets) != 1 {
		t.Fatalf("FC2 magnets = %v, err = %v", magnets, err)
	}
}

const compilationFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://sukebei.nyaa.si/xmlns/nyaa"><channel>
<item><title>SSIS-001 4時間 BEST</title><nyaa:infoHash>3333333333333333333333333333333333333333</nyaa:infoHash><nyaa:size>12 GiB</nyaa:size></item>
</channel></rss>`

func TestFindSkipsCompilations(t *testing.T) {
	fake := &fakeHTTP{body: compilationFeed, status: 200}
	client := NewForTest(fake)
	defer client.Close()

	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil || len(magnets) != 0 {
		t.Fatalf("compilation should be skipped: magnets = %v, err = %v", magnets, err)
	}
}
