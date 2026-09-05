package javdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	http "github.com/bogdanfinn/fhttp"
)

// Media is a raw image or video resource served by JavDB's CDN.
type Media struct {
	ContentType string
	Body        []byte
}

// FetchMedia downloads a JavDB CDN resource through the App transport. The
// browser cannot fetch these hosts directly in every network environment.
func (c *Client) FetchMedia(ctx context.Context, rawURL string) (Media, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return Media{}, errors.New("JavDB media URL must be an absolute https URL")
	}

	state, err := c.ensureRoute(ctx)
	if err != nil {
		return Media{}, err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return Media{}, err
	}
	return state.transport.getMedia(ctx, rawURL)
}

func (t *transport) getMedia(ctx context.Context, rawURL string) (Media, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Media{}, fmt.Errorf("create JavDB media request: %w", err)
	}
	request.Header.Set("user-agent", userAgent)

	response, err := t.client.Do(request)
	if err != nil {
		return Media{}, &networkError{err: err}
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Media{}, &HTTPError{StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Media{}, &networkError{err: fmt.Errorf("read JavDB media: %w", err)}
	}
	return Media{ContentType: response.Header.Get("content-type"), Body: body}, nil
}
