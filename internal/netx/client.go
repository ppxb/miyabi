package netx

import (
	"math"
	"net/http"
	"net/url"
	"time"

	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/go-resty/resty/v2"
)

// RestyOptions configures a standard-library backed HTTP client.
type RestyOptions struct {
	// Timeout bounds the whole request; zero disables it.
	Timeout time.Duration
	// ResponseHeaderTimeout bounds waiting for response headers only, which
	// suits long media transfers that must not have a total timeout.
	ResponseHeaderTimeout time.Duration
}

// NewRestyClient creates a client whose proxy is resolved from the manager on
// every request, so configuration changes apply without rebuilding it.
func NewRestyClient(manager *ProxyManager, options RestyOptions) *resty.Client {
	transport := newTransport(options)
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return manager.Resolve(), nil
	}
	return resty.New().SetTimeout(options.Timeout).SetTransport(transport)
}

// NewDirectRestyClient creates a client that never reads the proxy manager.
func NewDirectRestyClient(options RestyOptions) *resty.Client {
	return resty.New().SetTimeout(options.Timeout).SetTransport(newTransport(options))
}

func newTransport(options RestyOptions) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = options.ResponseHeaderTimeout
	return transport
}

// FingerprintOptions configures a browser-fingerprinted client. The proxy is
// fixed at construction because tls-client cannot change it afterwards.
type FingerprintOptions struct {
	Timeout   time.Duration
	Proxy     *url.URL
	CookieJar bool
}

// NewFingerprintClient creates a Chrome-fingerprinted client that never
// follows redirects.
func NewFingerprintClient(options FingerprintOptions) (tlsclient.HttpClient, error) {
	clientOptions := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(int(math.Ceil(options.Timeout.Seconds()))),
		tlsclient.WithClientProfile(profiles.Chrome_120),
		tlsclient.WithNotFollowRedirects(),
	}
	if options.CookieJar {
		clientOptions = append(clientOptions, tlsclient.WithCookieJar(tlsclient.NewCookieJar()))
	}
	if options.Proxy != nil {
		clientOptions = append(clientOptions, tlsclient.WithProxyUrl(options.Proxy.String()))
	}
	return tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), clientOptions...)
}
