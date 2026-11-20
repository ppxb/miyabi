package javdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
)

// AccountSettingKey stores the optional JavDB account used for search.
const AccountSettingKey = "javdb.account"

// ErrLoginRequired is returned when a gated request needs a configured account.
var ErrLoginRequired = domain.E(domain.KindUnauthorized, "JavDB 搜索需要登录，请先在「设置 → JavDB 账号」配置账号", nil)

// Config holds the optional JavDB account credentials.
type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Normalize trims inputs and requires both fields together.
func (c *Config) Normalize() error {
	c.Username = strings.TrimSpace(c.Username)
	c.Password = strings.TrimSpace(c.Password)
	if (c.Username == "") != (c.Password == "") {
		return domain.E(domain.KindInvalid, "JavDB 用户名和密码需同时填写", nil)
	}
	return nil
}

// Configured reports whether a username and password are available.
func (c Config) Configured() bool { return c.Username != "" && c.Password != "" }

// LoginInfo describes the authenticated account after a successful login.
type LoginInfo struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

// SetCredentials updates the stored account and clears any cached token.
func (c *Client) SetCredentials(username, password string) {
	c.authMu.Lock()
	c.username = strings.TrimSpace(username)
	c.password = strings.TrimSpace(password)
	c.token = ""
	c.authMu.Unlock()
	c.webFallback.Store(false)
}

// Credentials returns the stored account.
func (c *Client) Credentials() (string, string) {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.username, c.password
}

func (c *Client) accessToken() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.token
}

// Login authenticates with the App API and stores the bearer token. Detail and
// search then use the authenticated endpoints instead of the web fallback.
func (c *Client) Login(ctx context.Context, username, password string) (LoginInfo, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return LoginInfo{}, domain.E(domain.KindInvalid, "请填写 JavDB 用户名和密码", nil)
	}
	if c.fingerprint == nil {
		return LoginInfo{}, errors.New("JavDB web client is unavailable")
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return LoginInfo{}, err
	}
	form := url.Values{"username": {username}, "password": {password}}
	endpoint := webBaseURL + "/api/v1/sessions?" + publicParams(c.options.DeviceUUID).Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return LoginInfo{}, fmt.Errorf("create JavDB login request: %w", err)
	}
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("accept-language", defaultLanguage)
	request.Header.Set("jdsignature", signature(time.Now().Unix()))
	request.Header.Set("user-agent", userAgent)

	response, err := c.fingerprint.Do(request)
	if err != nil {
		return LoginInfo{}, &networkError{err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, 4096)
		return LoginInfo{}, &HTTPError{StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return LoginInfo{}, &networkError{err: fmt.Errorf("read JavDB login response: %w", err)}
	}
	var data struct {
		Token       string          `json:"token"`
		AccessToken string          `json:"access_token"`
		User        json.RawMessage `json:"user"`
	}
	if err := decodeEnvelope(body, &data); err != nil {
		return LoginInfo{}, err
	}
	token := data.Token
	if token == "" {
		token = data.AccessToken
	}
	if token == "" {
		return LoginInfo{}, errors.New("JavDB 登录响应缺少 token")
	}
	c.authMu.Lock()
	c.username, c.password, c.token = username, password, token
	c.authMu.Unlock()
	c.webFallback.Store(false)
	return loginInfo(username, data.User), nil
}

// getJSONWithAuth replays a gated request once after logging in, when an
// account is configured. Without credentials the original error is returned so
// callers can fall back to the public web pages.
func (c *Client) getJSONWithAuth(ctx context.Context, path string, params url.Values, destination any) error {
	err := c.getJSON(ctx, path, params, destination)
	if err == nil || !requiresWebFallback(err) {
		return err
	}
	username, password := c.Credentials()
	if username == "" || password == "" {
		return err
	}
	if _, loginErr := c.Login(ctx, username, password); loginErr != nil {
		return loginErr
	}
	return c.getJSON(ctx, path, params, destination)
}

func loginInfo(username string, raw json.RawMessage) LoginInfo {
	info := LoginInfo{Username: username}
	if len(raw) == 0 || string(raw) == "null" {
		return info
	}
	var user map[string]any
	if json.Unmarshal(raw, &user) != nil {
		return info
	}
	if id, ok := user["id"]; ok {
		switch value := id.(type) {
		case float64:
			info.UserID = strconv.FormatFloat(value, 'f', -1, 64)
		case string:
			info.UserID = value
		default:
			info.UserID = fmt.Sprint(value)
		}
	}
	for _, key := range []string{"name", "username", "nickname"} {
		if value, ok := user[key].(string); ok && value != "" {
			info.Username = value
			break
		}
	}
	return info
}
