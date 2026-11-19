package providers

import (
	"context"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

// MuraMura adapts muramura.tv, another 1pondo-family JSON source.
type MuraMura struct{ client }

func NewMuraMura(proxy *netx.ProxyManager) *MuraMura {
	return &MuraMura{newClient(proxy, "https://www.muramura.tv/", "", "muramura.tv")}
}

func (*MuraMura) ID() string { return "muramura" }

var muraMuraCode = regexp.MustCompile(`^\d{6}[-_]\d{3}$`)

func (*MuraMura) Supports(code string) bool { return muraMuraCode.MatchString(code) }

func (s *MuraMura) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !muraMuraCode.MatchString(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	movieID := strings.ReplaceAll(code, "-", "_")
	body, err := s.get(ctx, s.base+"dyn/phpauto/movie_details/movie_id/"+movieID+".json")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parsePondoMovieJSON(body, code, movieID, "muramura", 2476)
}
