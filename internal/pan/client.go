package pan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

const (
	passportURL = "https://passportapi.115.com"
	qrcodeURL   = "https://qrcodeapi.115.com"
	apiURL      = "https://proapi.115.com"
)

type Client struct {
	http    *resty.Client
	media   *resty.Client
	limiter *rate.Limiter
}

const (
	// requestTimeout bounds one API call; media transfers only bound the
	// wait for response headers because video bodies stream for hours.
	requestTimeout = 35 * time.Second
	// requestGap keeps the client under the 4 req/s that 115 tolerates safely.
	requestGap = 250 * time.Millisecond
	// maxInFlight bounds concurrent in-flight requests to avoid tripping 115 risk control.
	maxInFlight = 2
)

type panTransport struct {
	base       http.RoundTripper
	limiter    *rate.Limiter
	inFlight   chan struct{}
	mu         sync.Mutex
	retryAfter time.Time
}

func newPanTransport(base http.RoundTripper, limiter *rate.Limiter, maxConcurrent int) *panTransport {
	if maxConcurrent <= 0 {
		maxConcurrent = maxInFlight
	}
	return &panTransport{
		base:     base,
		limiter:  limiter,
		inFlight: make(chan struct{}, maxConcurrent),
	}
}

func (t *panTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "__EMPTY__" {
		req.Header.Del("User-Agent")
		req.Header["User-Agent"] = []string{""}
	}
	// 1. Limit concurrent in-flight requests
	select {
	case t.inFlight <- struct{}{}:
		defer func() { <-t.inFlight }()
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}

	// 2. Comply with global Retry-After backoff window
	t.mu.Lock()
	blockRemaining := time.Until(t.retryAfter)
	t.mu.Unlock()
	if blockRemaining > 0 {
		select {
		case <-time.After(blockRemaining):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	// 3. Enforce rate limit before every attempt, including all internal retries
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}

	// 4. Perform actual network call
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil {
		if wait := parseRetryAfter(resp.Header.Get("Retry-After")); wait > 0 {
			t.mu.Lock()
			until := time.Now().Add(wait)
			if until.After(t.retryAfter) {
				t.retryAfter = until
			}
			t.mu.Unlock()
		}
	}
	return resp, err
}

func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		wait := time.Until(t)
		if wait > 0 {
			return wait
		}
	}
	return 0
}

// New creates a 115 client. 115 is always reached directly: routing it through
// the upstream proxy is slower and trips risk control.
func New() *Client {
	httpClient := netx.NewDirectRestyClient(netx.RestyOptions{Timeout: requestTimeout})
	limiter := rate.NewLimiter(rate.Every(requestGap), 1)

	baseTransport := httpClient.GetClient().Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	transport := newPanTransport(baseTransport, limiter, maxInFlight)
	httpClient.SetTransport(transport)

	httpClient.SetRetryCount(3)
	httpClient.SetRetryWaitTime(1 * time.Second)
	// Allow sufficient max wait time so Resty does not truncate Retry-After headers (e.g. Retry-After: 60)
	httpClient.SetRetryMaxWaitTime(120 * time.Second)
	httpClient.SetRetryAfter(func(client *resty.Client, resp *resty.Response) (time.Duration, error) {
		if resp == nil {
			return 0, nil
		}
		wait := parseRetryAfter(resp.Header().Get("Retry-After"))
		if wait > 0 {
			return wait, nil
		}
		return 0, nil
	})
	httpClient.AddRetryCondition(func(r *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		if r != nil {
			status := r.StatusCode()
			return status == http.StatusBadGateway ||
				status == http.StatusServiceUnavailable ||
				status == http.StatusGatewayTimeout ||
				status == http.StatusTooManyRequests
		}
		return false
	})

	return &Client{
		http:    httpClient,
		media:   netx.NewDirectRestyClient(netx.RestyOptions{ResponseHeaderTimeout: requestTimeout}),
		limiter: limiter,
	}
}

func (client *Client) Close() {
	client.http.GetClient().CloseIdleConnections()
	client.media.GetClient().CloseIdleConnections()
}

func (client *Client) request(request *resty.Request, method, endpoint string) (*resty.Response, error) {
	response, err := request.Execute(method, endpoint)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("115 returned HTTP %d", response.StatusCode())
	}
	return response, nil
}

// Passport and QR polling use a numeric state; the file API uses a boolean.
type authResponse[T any] struct {
	State   int    `json:"state"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Errno   int    `json:"errno"`
	Data    T      `json:"data"`
}

func authRequest[T any](client *Client, request *resty.Request, method, endpoint string) (T, error) {
	var result authResponse[T]
	response, err := client.request(request, method, endpoint)
	if err != nil {
		return result.Data, err
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return result.Data, fmt.Errorf("decode 115 authorization response: %w", err)
	}
	if result.Error != "" {
		return result.Data, &apiError{Code: result.Errno, Message: result.Error}
	}
	if result.State != 1 || result.Code != 0 {
		return result.Data, &apiError{Code: result.Code, Message: result.Message}
	}
	return result.Data, nil
}

type apiResponse struct {
	State   bool   `json:"state"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (response apiResponse) err() error {
	if !response.State || response.Code != 0 {
		return &apiError{Code: response.Code, Message: response.Message}
	}
	return nil
}

type apiPayload interface {
	err() error
}

func apiRequest[T apiPayload](client *Client, request *resty.Request, method, endpoint, action string) (T, error) {
	var result T
	response, err := client.request(request, method, endpoint)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return result, fmt.Errorf("decode 115 %s: %w", action, err)
	}
	if err := result.err(); err != nil {
		return result, err
	}
	return result, nil
}
