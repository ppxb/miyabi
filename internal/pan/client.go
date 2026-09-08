package pan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/time/rate"
)

const (
	passportURL = "https://passportapi.115.com"
	qrcodeURL   = "https://qrcodeapi.115.com"
	apiURL      = "https://proapi.115.com"
)

type Options struct {
	Proxy string
}

type Client struct {
	http    *resty.Client
	limiter *rate.Limiter
}

func New(options Options) *Client {
	client := resty.New().SetTimeout(35 * time.Second)
	if options.Proxy != "" {
		client.SetProxy(options.Proxy)
	}
	return &Client{
		http:    client,
		limiter: rate.NewLimiter(rate.Every(500*time.Millisecond), 1),
	}
}

func (client *Client) Close() {
	client.http.GetClient().CloseIdleConnections()
}

func (client *Client) request(request *resty.Request, method, endpoint string) (*resty.Response, error) {
	if err := client.limiter.Wait(request.Context()); err != nil {
		return nil, err
	}
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
