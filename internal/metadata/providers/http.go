package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

type client struct {
	http          *http.Client
	limiter       *rate.Limiter
	base          string
	cookie        string
	hosts         []string
	cooldownMu    sync.Mutex
	cooldownUntil time.Time
	cooldownError error
}

func (c *client) Close() { c.http.CloseIdleConnections() }

func (c *client) cooldown() error {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()
	if wait := time.Until(c.cooldownUntil); wait > 0 {
		return &domain.RetryError{Cause: c.cooldownError, After: wait}
	}
	return nil
}

func (c *client) admit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.cooldown(); err != nil {
		return err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.cooldown()
}

func (c *client) failed(err error) error {
	if wait, retry := domain.RetryDelay(err); retry {
		c.cooldownMu.Lock()
		until := time.Now().Add(max(wait, 15*time.Second))
		if until.After(c.cooldownUntil) {
			c.cooldownUntil, c.cooldownError = until, err
		}
		defer c.cooldownMu.Unlock()
		return &domain.RetryError{Cause: err, After: time.Until(c.cooldownUntil)}
	}
	return err
}

func newClient(proxy *netx.ProxyManager, base, cookie string, hosts ...string) client {
	return client{http: netx.NewSafeDownloadClient(proxy, 20*time.Second), limiter: rate.NewLimiter(2, 2), base: base, cookie: cookie, hosts: hosts}
}

func (c *client) get(ctx context.Context, target string) ([]byte, error) {
	if err := c.admit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", c.base)
	if strings.HasPrefix(target, c.base) {
		req.Header.Set("Cookie", c.cookie)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, c.failed(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, metadata.ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, c.failed(&domain.HTTPError{Source: "metadata", StatusCode: response.StatusCode, RetryAfter: domain.ParseRetryAfter(response.Header.Get("Retry-After"), time.Now())})
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	if err != nil {
		return nil, c.failed(err)
	}
	if len(body) > 16<<20 {
		return nil, fmt.Errorf("response exceeds 16 MiB")
	}
	return body, nil
}

func (c *client) Media(ctx context.Context, target string) (domain.Media, error) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return domain.Media{}, domain.E(domain.KindInvalid, "无效的来源图片地址", err)
	}
	allowed := false
	for _, host := range c.hosts {
		if u.Hostname() == host || strings.HasSuffix(u.Hostname(), "."+host) {
			allowed = true
			break
		}
	}
	if !allowed {
		return domain.Media{}, domain.E(domain.KindInvalid, "图片地址不属于此来源", nil)
	}
	body, err := c.get(ctx, target)
	if err != nil {
		return domain.Media{}, err
	}
	return domain.Media{Body: body, ContentType: http.DetectContentType(body)}, nil
}
