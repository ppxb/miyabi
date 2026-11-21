package freejavbt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

const (
	defaultBaseURL  = "https://freejavbt.com"
	defaultTimeout  = 15 * time.Second
	defaultCacheTTL = 5 * time.Minute
	defaultCacheCap = 256
	maxBodyBytes    = 8 << 20
	userAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Options configures a freejavbt client.
type Options struct {
	Timeout   time.Duration
	Proxy     *netx.ProxyManager
	BaseURL   string
	Client    *http.Client
	CacheTTL  time.Duration
	CacheSize int
}

type cacheEntry struct {
	detail  detail
	expires time.Time
}

// Client implements both metadata.Source and magnet.Source for freejavbt.com.
// It caches parsed detail pages so metadata and magnet lookups for the same code
// share one HTTP request.
type Client struct {
	http      *http.Client
	limiter   *rate.Limiter
	base      string
	cacheTTL  time.Duration
	cacheSize int

	mu    sync.Mutex
	cache map[string]cacheEntry
	group singleflight.Group
}

// New creates a freejavbt client.
func New(options Options) (*Client, error) {
	base := strings.TrimRight(options.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	cacheTTL := options.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	cacheSize := options.CacheSize
	if cacheSize <= 0 {
		cacheSize = defaultCacheCap
	}
	httpClient := options.Client
	if httpClient == nil {
		httpClient = netx.NewSafeDownloadClient(options.Proxy, timeout)
	}
	return &Client{
		http:      httpClient,
		limiter:   rate.NewLimiter(2, 2),
		base:      base,
		cacheTTL:  cacheTTL,
		cacheSize: cacheSize,
		cache:     make(map[string]cacheEntry),
	}, nil
}

// ID identifies the metadata source.
func (*Client) ID() string { return "freejavbt" }

// Name identifies the magnet source.
func (*Client) Name() string { return domain.MagnetSourceFreeJavBT }

// Supports reports whether the code can be looked up.
func (*Client) Supports(code string) bool { return codeid.Normalize(code) != "" }

// Close releases idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Referer", c.base+"/")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "freejavbt 请求失败", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, metadata.ErrNotFound
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusServiceUnavailable {
		return nil, domain.Blocked(fmt.Sprintf("freejavbt 返回状态 %d", response.StatusCode))
	}
	if response.StatusCode != http.StatusOK {
		return nil, domain.E(domain.KindUpstream, fmt.Sprintf("freejavbt 返回状态 %d", response.StatusCode), nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "读取 freejavbt 响应失败", err)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("freejavbt 响应超过 %d 字节", maxBodyBytes)
	}
	if netx.IsCloudflareInterstitial(body) {
		return nil, domain.Blocked("freejavbt 遇到 Cloudflare 挑战")
	}
	return body, nil
}

func (c *Client) detail(ctx context.Context, code string) (detail, error) {
	if cached, ok := c.cached(code); ok {
		return cached, nil
	}
	value, err, _ := c.group.Do(code, func() (any, error) {
		if cached, ok := c.cached(code); ok {
			return cached, nil
		}
		// The flight is shared by every caller for this code, so it must not be
		// cancelled by whichever caller happened to start it. The HTTP client's
		// timeout still bounds the request.
		body, err := c.get(context.WithoutCancel(ctx), c.base+"/"+url.PathEscape(code))
		if err != nil {
			return detail{}, err
		}
		parsed := parseDetail(string(body))
		if parsed.Title == "" || (parsed.Code != "" && !codeid.IsFormatEquivalent(parsed.Code, code)) {
			return detail{}, metadata.ErrNotFound
		}
		c.store(code, parsed)
		return parsed, nil
	})
	if err != nil {
		return detail{}, err
	}
	return value.(detail), nil
}

func (c *Client) cached(code string) (detail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[code]
	if !ok || time.Now().After(entry.expires) {
		return detail{}, false
	}
	return entry.detail, true
}

func (c *Client) store(code string, parsed detail) {
	if c.cacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheSize > 0 && len(c.cache) >= c.cacheSize {
		for key := range c.cache {
			delete(c.cache, key)
			break
		}
	}
	c.cache[code] = cacheEntry{detail: parsed, expires: time.Now().Add(c.cacheTTL)}
}

// Fetch resolves one movie's metadata.
func (c *Client) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !c.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	parsed, err := c.detail(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	movie := domain.Movie{
		Code:          code,
		Title:         parsed.Title,
		ReleaseDate:   parsed.ReleaseDate,
		Duration:      parsed.Duration,
		Cover:         parsed.Cover,
		Thumbnail:     parsed.Cover,
		PreviewImages: parsed.Previews,
		HasPreview:    len(parsed.Previews) > 0,
		Sources:       []domain.SourceID{{Provider: "freejavbt", ID: code}},
	}
	if len(parsed.Actors) > 0 {
		movie.Actors = make([]domain.Actor, len(parsed.Actors))
		for index, name := range parsed.Actors {
			movie.Actors[index] = domain.Actor{Provider: "freejavbt", ID: name, Name: name}
		}
	}
	if len(parsed.Tags) > 0 {
		movie.Tags = make([]domain.Tag, len(parsed.Tags))
		for index, name := range parsed.Tags {
			movie.Tags[index] = domain.Tag{Provider: "freejavbt", ID: name, Name: name}
		}
	}
	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie, Zone: domain.ZoneUnknown}}
	result.Detail.ActorMovies = parsed.ActorMovies
	result.Detail.RelatedMovies = parsed.RelatedMovies
	if parsed.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "freejavbt", URL: parsed.Cover, Role: "cover"})
	}
	for _, preview := range parsed.Previews {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "freejavbt", URL: preview.Original, Role: "preview"})
	}
	return result, nil
}

// Media fetches a cover or preview image hosted by this source.
func (c *Client) Media(ctx context.Context, target string) (domain.Media, error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return domain.Media{}, domain.E(domain.KindInvalid, "无效的来源图片地址", nil)
	}
	host := parsed.Hostname()
	allowed := host == "jdbstatic.com" || strings.HasSuffix(host, ".jdbstatic.com") || host == "666.9989641.xyz"
	if !allowed {
		return domain.Media{}, domain.E(domain.KindInvalid, "图片地址不属于此来源", nil)
	}
	body, err := c.get(ctx, target)
	if err != nil {
		return domain.Media{}, err
	}
	return domain.Media{Body: body, ContentType: http.DetectContentType(body)}, nil
}

// Find returns the magnets rendered on the movie page.
func (c *Client) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" {
		return nil, nil
	}
	parsed, err := c.detail(ctx, code)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	magnets := make([]domain.Magnet, 0, len(parsed.Magnets))
	for _, entry := range parsed.Magnets {
		magnets = append(magnets, domain.Magnet{
			Hash:        entry.Hash,
			Name:        entry.Name,
			HasSubtitle: entry.HasSubtitle,
			HD:          entry.HD,
			FilesCount:  entry.FilesCount,
			CreatedAt:   entry.CreatedAt,
			Sources:     []string{domain.MagnetSourceFreeJavBT},
		})
	}
	return magnets, nil
}
