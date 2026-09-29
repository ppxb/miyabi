package javdb

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/netx"
)

type transport struct {
	host       string
	client     netx.FingerprintHTTPClient
	deviceUUID string
}

// routeTransport uses the managed client for normal API requests. Probe
// transports remain independent and retain the proxy they were created with.
func routeTransport(host string, client netx.FingerprintHTTPClient, options Options) (*transport, error) {
	host, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	return &transport{host: host, client: client, deviceUUID: options.DeviceUUID}, nil
}

type networkError struct {
	err error
}

func (e *networkError) Error() string {
	return e.err.Error()
}

func (e *networkError) Unwrap() error {
	return e.err
}

func (e *networkError) DomainKind() domain.Kind { return domain.KindUpstream }

func (e *networkError) PublicMessage() string {
	return "无法连接 JavDB，请检查网络代理或线路设置"
}

// newTransport builds an independent client for a route probe. Its proxy stays
// fixed for the duration of that measurement.
func newTransport(host string, proxy *url.URL, options Options) (*transport, error) {
	host, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	client, err := netx.NewFingerprintClient(netx.FingerprintOptions{
		Timeout: options.Timeout, Proxy: proxy, CookieJar: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create JavDB transport: %w", err)
	}
	return &transport{host: host, client: client, deviceUUID: options.DeviceUUID}, nil
}

func (t *transport) closeIdleConnections() {
	t.client.CloseIdleConnections()
}

const maxResponseBodyBytes = 4 << 20 // 4 MiB

func (t *transport) getJSON(
	ctx context.Context,
	path string,
	params url.Values,
	language string,
	destination any,
) error {
	request, err := t.newRequest(ctx, path, params, language, time.Now().Unix())
	if err != nil {
		return err
	}
	response, err := t.client.Do(request)
	if err != nil {
		return &networkError{err: err}
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, 4096)
		return &HTTPError{StatusCode: response.StatusCode, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}

	limited := io.LimitReader(response.Body, maxResponseBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return &networkError{err: fmt.Errorf("read JavDB response: %w", err)}
	}
	if int64(len(body)) > maxResponseBodyBytes {
		return &networkError{err: fmt.Errorf("JavDB response exceeded %d bytes limit", maxResponseBodyBytes)}
	}
	return decodeEnvelope(body, destination)
}

func (t *transport) newRequest(
	ctx context.Context,
	path string,
	params url.Values,
	language string,
	timestamp int64,
) (*http.Request, error) {
	query := url.Values{
		"app_channel":        {"official"},
		"app_version":        {appVersion},
		"app_version_number": {appVersionNumber},
		"platform":           {"android"},
		"system_version":     {"13"},
		"device_model":       {"Pixel 6"},
		"device_name":        {"Pixel"},
		"device_uuid":        {t.deviceUUID},
	}
	for key, values := range params {
		for _, value := range values {
			query.Add(key, value)
		}
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		t.host+"/"+strings.TrimLeft(path, "/")+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create JavDB request: %w", err)
	}
	if language == "" {
		language = defaultLanguage
	}
	request.Header.Set("accept-language", language)
	request.Header.Set("connection", "keep-alive")
	request.Header.Set("jdsignature", signature(timestamp))
	request.Header.Set("user-agent", userAgent)
	return request, nil
}
