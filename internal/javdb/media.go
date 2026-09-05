package javdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-resty/resty/v2"
)

// Media is a decoded image served by JavDB's CDN.
type Media struct {
	ContentType string
	Body        []byte
}

func newMediaClient(options Options) *resty.Client {
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", userAgent).
		SetRedirectPolicy(resty.NoRedirectPolicy())
	if options.Proxy != "" {
		client.SetProxy(options.Proxy)
	}
	return client
}

// FetchMedia downloads and decodes a CDN image independently of API route
// selection and API rate limiting, using the same configured outbound proxy.
func (c *Client) FetchMedia(ctx context.Context, rawURL string) (Media, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return Media{}, errors.New("JavDB media URL must be an absolute https URL")
	}

	response, err := c.media.R().SetContext(ctx).Get(rawURL)
	if err != nil {
		return Media{}, fmt.Errorf("download JavDB image: %w", err)
	}
	if response.StatusCode() < 200 || response.StatusCode() >= 300 {
		return Media{}, &HTTPError{StatusCode: response.StatusCode()}
	}
	return decodeImagePayload(response.Body())
}

// The CDN serves either a standard image or a one-byte XOR key followed by the
// encoded image. Detect the decoded MIME type instead of forwarding octet-stream.
func decodeImagePayload(raw []byte) (Media, error) {
	contentType := http.DetectContentType(raw)
	if strings.HasPrefix(contentType, "image/") {
		return Media{ContentType: contentType, Body: raw}, nil
	}
	if len(raw) < 2 {
		return Media{}, errors.New("JavDB media response is not a recognized image")
	}

	key := raw[0]
	decoded := make([]byte, len(raw)-1)
	for index := range decoded {
		decoded[index] = raw[index+1] ^ key
	}
	contentType = http.DetectContentType(decoded)
	if !strings.HasPrefix(contentType, "image/") {
		return Media{}, errors.New("JavDB media response is not a recognized image")
	}
	return Media{ContentType: contentType, Body: decoded}, nil
}
