package javdb

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type transport struct {
	host       string
	client     httpClient
	deviceUUID string
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

func newTransport(host string, options Options) (*transport, error) {
	host, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	clientOptions := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(int(math.Ceil(timeout.Seconds()))),
		tlsclient.WithClientProfile(profiles.Chrome_120),
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
	}
	if options.Proxy != "" {
		clientOptions = append(clientOptions, tlsclient.WithProxyUrl(options.Proxy))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("create JavDB transport: %w", err)
	}
	return &transport{host: host, client: client, deviceUUID: options.DeviceUUID}, nil
}

func (t *transport) closeIdleConnections() {
	t.client.CloseIdleConnections()
}

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

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return &networkError{err: fmt.Errorf("read JavDB response: %w", err)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPError{StatusCode: response.StatusCode}
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
