package providers

import (
	"context"
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

// CaribbeancomPremium adapts www.caribbeancompr.com (EUC-JP). It shares the
// caribbeancom page layout but uses underscore date ids (052121_002).
type CaribbeancomPremium struct{ client }

func NewCaribbeancomPremium(proxy *netx.ProxyManager) *CaribbeancomPremium {
	return &CaribbeancomPremium{newClient(proxy, "https://www.caribbeancompr.com/", "", "caribbeancompr.com", "caribbeancom.com")}
}

func (*CaribbeancomPremium) ID() string { return "caribbeancompr" }

func (*CaribbeancomPremium) Supports(code string) bool { return caribbeanDateCode.MatchString(code) }

func (s *CaribbeancomPremium) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !caribbeanDateCode.MatchString(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	siteID := strings.ReplaceAll(code, "-", "_")
	body, err := s.get(ctx, s.base+"moviepages/"+url.PathEscape(siteID)+"/index.html")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseCaribbean(decodeHTML(body), s.base, code, siteID, "caribbeancompr", "カリビアンコムプレミアム")
}
