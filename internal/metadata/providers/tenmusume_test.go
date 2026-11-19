package providers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

const pondoMovieJSON = `{"MovieID":"042922_01","SiteID":2471,"Title":"サンプル作品","Desc":"あらすじ本文","Release":"2022-04-29","Duration":3600,"ThumbHigh":"https://www.10musume.com/moviepages/042922_01/images/str.jpg","ActressesJa":["女優A","女優B"],"UCNAME":["企画"],"Series":"シリーズX","AvgRating":3}`

func TestPondoMovieJSONParses(t *testing.T) {
	m, err := parsePondoMovieJSON([]byte(pondoMovieJSON), "042922_01", "042922_01", "10musume", 2471)
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "042922_01" || m.Detail.Title != "サンプル作品" || m.Detail.Plot != "あらすじ本文" {
		t.Fatalf("title/plot wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2022-04-29" || m.Detail.Duration != 60 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 2 || len(m.Detail.Tags) != 1 || m.Detail.Series == nil {
		t.Fatalf("actors/tags/series wrong: %+v", m.Detail)
	}
	if m.Detail.Rating != 3 || m.Detail.RatingMax != 5 || m.Detail.Zone != domain.ZoneUncensored {
		t.Fatalf("rating/zone wrong: %+v", m.Detail)
	}
	if m.Detail.Cover != "https://www.10musume.com/moviepages/042922_01/images/str.jpg" || len(m.Images) != 1 {
		t.Fatalf("cover wrong: %+v", m.Detail.Cover)
	}
	if _, err := parsePondoMovieJSON([]byte(`{"Duration":10}`), "042922_01", "042922_01", "10musume", 2471); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("missing title accepted: %v", err)
	}
	// Wrong MovieID / SiteID must be rejected even with a valid title.
	if _, err := parsePondoMovieJSON([]byte(`{"MovieID":"999999_99","SiteID":2471,"Title":"x"}`), "042922_01", "042922_01", "10musume", 2471); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("wrong MovieID accepted: %v", err)
	}
	if _, err := parsePondoMovieJSON([]byte(`{"MovieID":"042922_01","SiteID":2476,"Title":"x"}`), "042922_01", "042922_01", "10musume", 2471); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("wrong SiteID accepted: %v", err)
	}
}

func TestTenMusumeMuramuraSupports(t *testing.T) {
	t10 := &TenMusume{}
	mm := &MuraMura{}
	if !t10.Supports("042922_01") || t10.Supports("091522_959") || t10.Supports("SSIS-033") {
		t.Fatal("10musume Supports wrong")
	}
	if !mm.Supports("091522_959") || mm.Supports("042922_01") || mm.Supports("SSIS-033") {
		t.Fatal("muramura Supports wrong")
	}
	if _, err := t10.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported 10musume fetched: %v", err)
	}
	if _, err := mm.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported muramura fetched: %v", err)
	}
}

func livePondo(t *testing.T, env string, newSrc func(*netx.ProxyManager) interface {
	Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
	Close()
}) {
	t.Helper()
	codes := os.Getenv(env)
	if codes == "" {
		t.Skip("set " + env + " for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := newSrc(proxy)
	defer s.Close()
	for _, c := range strings.Split(codes, ",") {
		ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
		m, err := s.Fetch(ctx, domain.MovieRef{Code: c})
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if m.Detail.Code == "" || m.Detail.Title == "" || m.Detail.Cover == "" {
			t.Fatalf("%s missing core metadata: %+v", c, m.Detail)
		}
		t.Logf("%s: release %q dur %d actors %d tags %d", c, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors), len(m.Detail.Tags))
	}
}

func TestTenMusumeLive(t *testing.T) {
	livePondo(t, "MIYABI_TEST_TENMUSUME_CODES", func(p *netx.ProxyManager) interface {
		Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
		Close()
	} {
		return NewTenMusume(p)
	})
}

func TestMuramuraLive(t *testing.T) {
	livePondo(t, "MIYABI_TEST_MURAMURA_CODES", func(p *netx.ProxyManager) interface {
		Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
		Close()
	} {
		return NewMuraMura(p)
	})
}
