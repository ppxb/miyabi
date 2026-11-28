// Package guangya implements the private Guangya (光鸭云盘) client used by the
// storage provider. It intentionally mirrors the web client: login is SMS based
// and business requests use a browser device identity.
package guangya

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/time/rate"
)

const (
	accountBase = "https://account.guangyapan.com"
	apiBase     = "https://api.guangyapan.com"
	clientID    = "aMe-8VSlkrbQXpUR"
	userAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
	maxBody = 16 << 20
)

// Client implements drive.Client for Guangya. PickCode is used as an opaque
// file handle: Guangya sets it to the file id because its APIs are id-based.
type Client struct {
	http        *http.Client
	media       *http.Client
	limiter     *rate.Limiter
	did         string
	accountBase string
	apiBase     string

	mu    sync.Mutex
	names map[string]string
}

func New() *Client { return NewWithBases(accountBase, apiBase) }

// NewWithBases allows tests to point the client at a local server.
func NewWithBases(accountURL, apiURL string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 35 * time.Second},
		media:       &http.Client{},
		limiter:     rate.NewLimiter(rate.Limit(5), 5),
		did:         newDeviceID(),
		accountBase: accountURL,
		apiBase:     apiURL,
		names:       make(map[string]string),
	}
}

func (c *Client) Close() {}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return fmt.Sprintf("guangya error %d: %s", e.code, e.msg) }

func (e *apiError) PublicMessage() string { return e.msg }

func (e *apiError) DomainKind() domain.Kind {
	switch {
	case e.Is(pan.ErrUnauthorized):
		return domain.KindUnauthorized
	case e.Is(pan.ErrNotFound):
		return domain.KindNotFound
	case e.Is(pan.ErrOfflineExists):
		return domain.KindConflict
	default:
		return domain.KindUpstream
	}
}

func (e *apiError) Is(target error) bool {
	switch target {
	case pan.ErrUnauthorized:
		return e.code == 117 || e.code == 401
	case pan.ErrNotFound:
		return e.code == 146 || e.code == 149
	case pan.ErrDownloadUnavailable:
		return e.code == 254
	case pan.ErrOfflineExists:
		msg := strings.ToLower(e.msg)
		return e.code == 10008 || strings.Contains(msg, "已存在") ||
			strings.Contains(msg, "exist") || strings.Contains(msg, "duplicate")
	default:
		return false
	}
}

func (e *envelope) err() error {
	if e == nil || e.Code == 0 {
		return nil
	}
	return &apiError{code: e.Code, msg: e.Msg}
}

func (c *Client) do(ctx context.Context, method, address string, headers map[string]string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return 0, nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return 0, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, domain.E(domain.KindUpstream, "光鸭请求失败", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return 0, nil, domain.E(domain.KindUpstream, "读取光鸭响应失败", err)
	}
	return resp.StatusCode, raw, nil
}

func (c *Client) call(ctx context.Context, method, address string, headers map[string]string, body any, out any) error {
	status, raw, err := c.do(ctx, method, address, headers, body)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return pan.ErrUnauthorized
	}
	if status < 200 || status >= 300 {
		return domain.E(domain.KindUpstream,
			fmt.Sprintf("光鸭返回 HTTP %d%s", status, upstreamDetail(raw)), nil)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return domain.E(domain.KindUpstream, "解析光鸭响应失败", err)
		}
	}
	return nil
}

func (c *Client) callAPI(ctx context.Context, token, path string, body any, out any) error {
	var env envelope
	if err := c.call(ctx, http.MethodPost, c.apiBase+path, c.businessHeaders(token), body, &env); err != nil {
		return err
	}
	if err := env.err(); err != nil {
		return err
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return domain.E(domain.KindUpstream, "解析光鸭业务数据失败", err)
		}
	}
	return nil
}

func (c *Client) accountCall(ctx context.Context, method, token, path string, body any, extra map[string]string) (int, []byte, error) {
	headers := c.accountHeaders()
	if token != "" {
		headers["authorization"] = "Bearer " + token
	}
	for key, value := range extra {
		headers[key] = value
	}
	return c.do(ctx, method, c.accountBase+path, headers, body)
}

func (c *Client) callAccount(ctx context.Context, method, token, path string, body any, out any, extra map[string]string) error {
	status, raw, err := c.accountCall(ctx, method, token, path, body, extra)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return pan.ErrUnauthorized
	}
	if status < 200 || status >= 300 {
		return domain.E(domain.KindUpstream,
			fmt.Sprintf("光鸭返回 HTTP %d%s", status, upstreamDetail(raw)), nil)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return domain.E(domain.KindUpstream, "解析光鸭响应失败", err)
		}
	}
	return nil
}

func (c *Client) accountHeaders() map[string]string {
	return map[string]string{
		"accept":             "*/*",
		"content-type":       "application/json",
		"origin":             "https://www.guangyapan.com",
		"referer":            "https://www.guangyapan.com/",
		"user-agent":         userAgent,
		"x-client-id":        clientID,
		"x-client-version":   "0.0.1",
		"x-device-id":        c.did,
		"x-device-model":     "chrome%2F147.0.0.0",
		"x-device-name":      "PC-Chrome",
		"x-device-sign":      "wdi10." + c.did + randomHex(16),
		"x-net-work-type":    "NONE",
		"x-os-version":       "MacIntel",
		"x-platform-version": "1",
		"x-protocol-version": "301",
		"x-provider-name":    "NONE",
		"x-sdk-version":      "9.0.2",
	}
}

func (c *Client) businessHeaders(token string) map[string]string {
	return map[string]string{
		"accept":        "application/json, text/plain, */*",
		"authorization": "Bearer " + token,
		"did":           c.did,
		"dt":            "4",
		"origin":        "https://www.guangyapan.com",
		"referer":       "https://www.guangyapan.com/",
		"user-agent":    userAgent,
		"traceparent":   traceparent(),
	}
}

func newDeviceID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return randomHex(16)
	}
	sum := md5.Sum(raw[:])
	return hex.EncodeToString(sum[:])
}

func randomHex(size int) string {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return strings.Repeat("0", size*2)
	}
	return hex.EncodeToString(raw)
}

func traceparent() string {
	return "00-" + randomHex(16) + "-" + randomHex(8) + "-01"
}

// Account returns the signed-in profile and storage usage.
func (c *Client) Account(ctx context.Context, accessToken string) (pan.Account, error) {
	var me struct {
		Sub  string `json:"sub"`
		Name string `json:"name"`
	}
	if err := c.callAccount(ctx, http.MethodGet, accessToken, "/v1/user/me", nil, &me, nil); err != nil {
		return pan.Account{}, err
	}
	if me.Sub == "" {
		return pan.Account{}, domain.E(domain.KindUpstream, "光鸭账号响应缺少用户 ID", nil)
	}
	name := me.Name
	if name == "" {
		name = me.Sub
	}
	account := pan.Account{ID: me.Sub, Name: name}
	space, err := c.space(ctx, accessToken)
	if err != nil {
		if errors.Is(err, pan.ErrUnauthorized) {
			return pan.Account{}, err
		}
		return account, nil
	}
	account.Space = space
	return account, nil
}

func (c *Client) space(ctx context.Context, accessToken string) (pan.AccountSpace, error) {
	var data struct {
		Total json.Number `json:"totalSpaceSize"`
		Used  json.Number `json:"usedSpaceSize"`
	}
	if err := c.callAPI(ctx, accessToken, "/nd.bizassets.s/v1/get_assets", map[string]any{}, &data); err != nil {
		return pan.AccountSpace{}, err
	}
	total := numberInt64(data.Total)
	used := numberInt64(data.Used)
	if total < 0 {
		total = 0
	}
	if used < 0 {
		used = 0
	}
	remaining := total - used
	if remaining < 0 {
		remaining = 0
	}
	return pan.AccountSpace{
		Total:     pan.SpaceAmount{Bytes: total, Formatted: formatBytes(total)},
		Used:      pan.SpaceAmount{Bytes: used, Formatted: formatBytes(used)},
		Remaining: pan.SpaceAmount{Bytes: remaining, Formatted: formatBytes(remaining)},
	}, nil
}

func upstreamDetail(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Msg              string `json:"msg"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if json.Unmarshal(raw, &payload) == nil {
		for _, value := range []string{payload.Msg, payload.Error, payload.ErrorDescription, payload.Message} {
			if value = strings.TrimSpace(value); value != "" {
				return ": " + truncate(value, 200)
			}
		}
	}
	text := strings.Join(strings.Fields(string(raw)), " ")
	if text == "" {
		return ""
	}
	return ": " + truncate(text, 200)
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func numberInt64(value json.Number) int64 {
	number, err := value.Int64()
	if err != nil {
		return 0
	}
	return number
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", bytes, units[index])
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}

func (c *Client) BeginLogin(context.Context) (*pan.Login, error) {
	return nil, domain.E(domain.KindInvalid, "光鸭使用扫码登录", nil)
}

func (c *Client) LoginStatus(context.Context, *pan.Login) (pan.LoginState, error) {
	return pan.LoginExpired, domain.E(domain.KindInvalid, "光鸭使用扫码登录", nil)
}

func (c *Client) ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error) {
	return pan.Tokens{}, domain.E(domain.KindInvalid, "光鸭使用扫码登录", nil)
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (pan.Tokens, error) {
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/auth/token", map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}, &result, map[string]string{"x-action": "401"})
	if err != nil {
		return pan.Tokens{}, err
	}
	if result.AccessToken == "" {
		return pan.Tokens{}, pan.ErrUnauthorized
	}
	return tokensOf(result.AccessToken, result.RefreshToken, result.ExpiresIn), nil
}

func tokensOf(accessToken, refreshToken string, expiresIn int64) pan.Tokens {
	tokens := pan.Tokens{AccessToken: accessToken, RefreshToken: refreshToken}
	if expiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	return tokens
}

// DeviceLogin is the device authorization response behind the website's QR
// login. VerificationURIComplete is the URL rendered as the QR code.
type DeviceLogin struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
	VerificationURI         string `json:"verification_url"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

// BeginDeviceLogin starts the same device authorization flow the website uses
// for QR login.
func (c *Client) BeginDeviceLogin(ctx context.Context) (DeviceLogin, error) {
	var result DeviceLogin
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/auth/device/code", map[string]string{
		"scope":     "user",
		"client_id": clientID,
	}, &result, nil)
	if err != nil {
		if errors.Is(err, pan.ErrUnauthorized) {
			return DeviceLogin{}, domain.E(domain.KindUpstream, "光鸭登录请求被拒绝", nil)
		}
		return DeviceLogin{}, err
	}
	if result.DeviceCode == "" || result.VerificationURIComplete == "" {
		return DeviceLogin{}, domain.E(domain.KindUpstream, "光鸭未返回扫码登录信息", nil)
	}
	return result, nil
}

// PollDeviceLogin performs one device authorization check. OAuth pending and
// slow_down responses are waiting states, not errors.
func (c *Client) PollDeviceLogin(ctx context.Context, deviceCode string) (pan.Tokens, pan.LoginState, error) {
	status, raw, err := c.accountCall(ctx, http.MethodPost, "", "/v1/auth/token", map[string]string{
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		"device_code": deviceCode,
		"client_id":   clientID,
	}, nil)
	if err != nil {
		return pan.Tokens{}, pan.LoginWaiting, err
	}
	var result struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil && status >= 200 && status < 300 {
			return pan.Tokens{}, pan.LoginWaiting,
				domain.E(domain.KindUpstream, "解析光鸭登录响应失败", err)
		}
	}
	switch result.Error {
	case "authorization_pending", "slow_down":
		return pan.Tokens{}, pan.LoginWaiting, nil
	case "access_denied":
		return pan.Tokens{}, pan.LoginCanceled, nil
	case "expired_token", "invalid_grant":
		return pan.Tokens{}, pan.LoginExpired, nil
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return pan.Tokens{}, pan.LoginWaiting,
			domain.E(domain.KindUpstream, "光鸭登录请求被拒绝", nil)
	}
	if status == http.StatusTooManyRequests {
		return pan.Tokens{}, pan.LoginWaiting,
			domain.E(domain.KindRateLimited, "光鸭登录请求过于频繁，请稍后再试", nil)
	}
	if status < 200 || status >= 300 {
		detail := strings.TrimSpace(result.ErrorDescription)
		if detail == "" {
			detail = strings.TrimSpace(result.Error)
		}
		message := "光鸭登录失败"
		if detail != "" {
			message += ": " + detail
		}
		return pan.Tokens{}, pan.LoginWaiting, domain.E(domain.KindUpstream, message, nil)
	}
	if result.AccessToken == "" {
		return pan.Tokens{}, pan.LoginWaiting,
			domain.E(domain.KindUpstream, "光鸭登录未返回令牌", nil)
	}
	return tokensOf(result.AccessToken, result.RefreshToken, result.ExpiresIn),
		pan.LoginAuthorized, nil
}

type fileItem struct {
	FileID       string      `json:"fileId"`
	ParentID     string      `json:"parentId"`
	Name         string      `json:"fileName"`
	Size         json.Number `json:"fileSize"`
	ResType      int         `json:"resType"`
	FileType     int         `json:"fileType"`
	Ext          string      `json:"ext"`
	CTime        int64       `json:"ctime"`
	UTime        int64       `json:"utime"`
	GCID         string      `json:"gcid"`
	FullParentID string      `json:"fullParentIds"`
}

func (item fileItem) size() int64 {
	value, err := item.Size.Int64()
	if err != nil {
		return 0
	}
	return value
}

func (item fileItem) directory() bool { return item.ResType == 2 }

func (item fileItem) file() pan.File {
	return pan.File{
		ID:          item.FileID,
		ParentID:    item.ParentID,
		Name:        item.Name,
		IsDirectory: item.directory(),
		Size:        item.size(),
		PickCode:    item.FileID,
		GCID:        item.GCID,
		UploadTime:  item.CTime,
	}
}

func (c *Client) List(ctx context.Context, accessToken, directoryID string, offset, limit int) (pan.FilePage, error) {
	if directoryID == "0" {
		directoryID = ""
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	page := offset / limit
	var data struct {
		List  []fileItem `json:"list"`
		Total int        `json:"total"`
	}
	err := c.callAPI(ctx, accessToken, "/userres/v1/file/get_file_list", map[string]any{
		"parentId": directoryID,
		"page":     page,
		"pageSize": limit,
		"orderBy":  0,
		"sortType": 0,
	}, &data)
	if err != nil {
		return pan.FilePage{}, err
	}
	result := pan.FilePage{
		Files:   make([]pan.File, len(data.List)),
		Total:   data.Total,
		HasMore: offset+len(data.List) < data.Total,
	}
	for i, item := range data.List {
		c.rememberName(item.FileID, item.Name)
		result.Files[i] = item.file()
	}
	result.Path = c.pathFor(ctx, accessToken, directoryID, data.List)
	return result, nil
}

func (c *Client) Info(ctx context.Context, accessToken, fileID string) (pan.FileInfo, error) {
	item, videoGCID, err := c.detail(ctx, accessToken, fileID)
	if err != nil {
		return pan.FileInfo{}, err
	}
	if item.FileID == "" {
		return pan.FileInfo{}, pan.ErrNotFound
	}
	if item.GCID == "" {
		item.GCID = videoGCID
	}
	c.rememberName(item.FileID, item.Name)
	file := item.file()
	file.PickCode = fileID
	return pan.FileInfo{
		File: file,
		Path: c.pathFromIDs(ctx, accessToken, splitIDs(item.FullParentID)),
	}, nil
}

type videoResource struct {
	GCID              string `json:"gcid"`
	MediaID           string `json:"media_id"`
	DefaultResolution bool   `json:"defaultResolution"`
	VideoResourceInfo `json:"info"`
}

type VideoResourceInfo struct {
	Source            json.Number `json:"source"`
	DefaultResolution bool        `json:"defaultResolution"`
	Resolution        *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"resolution"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (resource videoResource) id() string {
	if resource.GCID != "" {
		return resource.GCID
	}
	return resource.MediaID
}

func (resource videoResource) source() int {
	value, err := resource.Source.Int64()
	if err != nil {
		return 0
	}
	return int(value)
}

func (resource videoResource) defaultResolution() bool {
	return resource.DefaultResolution || resource.VideoResourceInfo.DefaultResolution
}

func (resource videoResource) pixels() int {
	if resource.Resolution != nil && resource.Resolution.Width > 0 && resource.Resolution.Height > 0 {
		return resource.Resolution.Width * resource.Resolution.Height
	}
	if resource.Width > 0 && resource.Height > 0 {
		return resource.Width * resource.Height
	}
	return int(^uint(0) >> 1)
}

// selectVideoGCID mirrors the web player: prefer a default source 1 resource,
// then a default source 0 resource, then the smallest resolution.
func selectVideoGCID(resources []videoResource) string {
	candidates := make([]videoResource, 0, len(resources))
	for _, resource := range resources {
		if (resource.source() == 0 || resource.source() == 1) && resource.id() != "" {
			candidates = append(candidates, resource)
		}
	}
	if len(candidates) == 0 {
		if len(resources) > 0 && resources[0].id() != "" {
			return resources[0].id()
		}
		return ""
	}
	for _, source := range []int{1, 0} {
		for _, resource := range candidates {
			if resource.source() == source && resource.defaultResolution() {
				return resource.id()
			}
		}
	}
	best := candidates[0]
	for _, resource := range candidates[1:] {
		if resource.pixels() < best.pixels() {
			best = resource
		}
	}
	return best.id()
}

func (c *Client) detail(ctx context.Context, accessToken, fileID string) (fileItem, string, error) {
	var data struct {
		FileInfo      fileItem        `json:"fileInfo"`
		VideoResource []videoResource `json:"videoResource"`
	}
	err := c.callAPI(ctx, accessToken, "/nd.bizuserres.s/v1/file/get_file_detail",
		map[string]string{"fileId": fileID}, &data)
	if err != nil {
		return fileItem{}, "", err
	}
	return data.FileInfo, selectVideoGCID(data.VideoResource), nil
}

func (c *Client) ReadMetadata(ctx context.Context, accessToken, pickCode string, limit int64) ([]byte, error) {
	address, err := c.downloadURL(ctx, accessToken, pickCode)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 2 << 20
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("range", fmt.Sprintf("bytes=0-%d", limit-1))
	req.Header.Set("user-agent", pan.MediaUserAgent)
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "读取光鸭文件失败", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, domain.E(domain.KindUpstream,
			fmt.Sprintf("读取光鸭文件返回 HTTP %d", resp.StatusCode), nil)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "读取光鸭文件内容失败", err)
	}
	return body, nil
}

// DownloadURL returns a direct media URL. Videos (identified by gcid) use the
// VOD endpoint directly; sidecar files use the ordinary download endpoint.
func (c *Client) DownloadURL(ctx context.Context, accessToken, pickCode, _ string) (string, error) {
	info, err := c.Info(ctx, accessToken, pickCode)
	if err != nil {
		return "", err
	}
	if info.GCID != "" {
		return c.vodURL(ctx, accessToken, pickCode, info.GCID)
	}
	return c.downloadURL(ctx, accessToken, pickCode)
}

func (c *Client) downloadURL(ctx context.Context, accessToken, fileID string) (string, error) {
	var data struct {
		SignedURL string `json:"signedURL"`
	}
	err := c.callAPI(ctx, accessToken, "/nd.bizuserres.s/v1/get_res_download_url",
		map[string]string{"fileId": fileID}, &data)
	if err != nil {
		return "", err
	}
	if data.SignedURL == "" {
		return "", pan.ErrDownloadUnavailable
	}
	return data.SignedURL, nil
}

func (c *Client) vodURL(ctx context.Context, accessToken, fileID, gcid string) (string, error) {
	var data struct {
		SignedURL string `json:"signedURL"`
	}
	body := map[string]string{"fileId": fileID, "gcid": gcid}
	// The web player uses the userres namespace. Keep the older OpenList
	// namespace as a fallback for accounts or deployments where it differs.
	err := c.callAPI(ctx, accessToken, "/userres/v1/file/get_vod_download_url", body, &data)
	if err != nil {
		err = c.callAPI(ctx, accessToken, "/nd.bizuserres.s/v1/file/get_vod_download_url", body, &data)
	}
	if err != nil {
		return "", err
	}
	if data.SignedURL == "" {
		return "", pan.ErrDownloadUnavailable
	}
	return data.SignedURL, nil
}

// PlayURL would be treated as HLS by the playback layer. Guangya's VOD URL is
// a direct mp4, so DownloadURL carries it instead.
func (c *Client) PlayURL(context.Context, string, string, string) ([]pan.PlaySource, error) {
	return nil, pan.ErrTranscodeUnavailable
}

type cloudTask struct {
	TaskID   string      `json:"taskId"`
	Name     string      `json:"fileName"`
	Size     json.Number `json:"totalSize"`
	Status   int         `json:"status"`
	Progress json.Number `json:"progress"`
	FileID   string      `json:"fileId"`
	IsDir    bool        `json:"isDir"`
	Res      string      `json:"res"`
}

func (c *Client) AddOffline(ctx context.Context, accessToken, uri, directoryID string) (string, error) {
	hash, ok := magnetHash(uri)
	if !ok {
		return "", domain.E(domain.KindInvalid, "不是有效的磁力链", nil)
	}
	if directoryID == "0" {
		directoryID = ""
	}
	var resolved struct {
		URL string `json:"url"`
	}
	if err := c.callAPI(ctx, accessToken, "/nd.bizcloudcollection.s/v1/resolve_res", map[string]string{
		"url": uri,
	}, &resolved); err != nil {
		return "", err
	}
	var data struct {
		TaskID string `json:"taskId"`
	}
	err := c.callAPI(ctx, accessToken, "/nd.bizcloudcollection.s/v1/create_task", map[string]string{
		"url":      uri,
		"parentId": directoryID,
	}, &data)
	if err != nil {
		if errors.Is(err, pan.ErrOfflineExists) {
			return "", pan.ErrOfflineExists
		}
		return "", err
	}
	if data.TaskID == "" {
		return "", domain.E(domain.KindUpstream, "光鸭未返回下载任务 ID", nil)
	}
	return hash, nil
}

func (c *Client) RemoveOffline(ctx context.Context, accessToken, hash string) error {
	tasks, err := c.cloudTasks(ctx, accessToken)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if strings.EqualFold(magnetHashOfTask(task), hash) {
			var result struct{}
			return c.callAPI(ctx, accessToken, "/nd.bizcloudcollection.s/v2/delete_task",
				map[string]any{"taskIds": []string{task.TaskID}}, &result)
		}
	}
	return nil
}

func (c *Client) OfflineTasks(ctx context.Context, accessToken string, page int) (pan.OfflinePage, error) {
	if page > 1 {
		return pan.OfflinePage{PageCount: 1}, nil
	}
	tasks, err := c.cloudTasks(ctx, accessToken)
	if err != nil {
		return pan.OfflinePage{}, err
	}
	result := pan.OfflinePage{PageCount: 1, Tasks: make([]pan.OfflineTask, 0, len(tasks))}
	for _, task := range tasks {
		hash := magnetHashOfTask(task)
		if hash == "" {
			continue
		}
		progress, hasProgress := numberFloat(task.Progress)
		status := pan.OfflineStatusWaiting
		switch task.Status {
		case 0:
			status = pan.OfflineStatusWaiting
		case 1:
			status = pan.OfflineStatusRunning
		case 2:
			status = pan.OfflineStatusDone
		case 3, 4, 5:
			status = pan.OfflineStatusFailed
		default:
			status = pan.OfflineStatusFailed
		}
		result.Tasks = append(result.Tasks, pan.OfflineTask{
			Hash:            hash,
			Status:          status,
			Progress:        int(clampProgress(progress)),
			RawProgress:     clampProgress(progress),
			ProgressUnknown: !hasProgress,
			FileID:          task.FileID,
		})
	}
	return result, nil
}

func (c *Client) cloudTasks(ctx context.Context, accessToken string) ([]cloudTask, error) {
	var all []cloudTask
	cursor := ""
	for {
		var data struct {
			List   []cloudTask `json:"list"`
			Cursor string      `json:"cursor"`
		}
		body := map[string]any{
			"status":   []int{0, 1, 2, 3, 4, 5},
			"cursor":   cursor,
			"pageSize": 50,
		}
		err := c.callAPI(ctx, accessToken, "/nd.bizcloudcollection.s/v1/list_task", body, &data)
		if err != nil {
			return nil, err
		}
		all = append(all, data.List...)
		if data.Cursor == "" || len(data.List) == 0 {
			return all, nil
		}
		cursor = data.Cursor
	}
}

func (c *Client) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if req.Header.Get("user-agent") == "" {
		req.Header.Set("user-agent", pan.MediaUserAgent)
	}
	return c.media.Do(req)
}

func (c *Client) rememberName(id, name string) {
	if id == "" || name == "" {
		return
	}
	c.mu.Lock()
	c.names[id] = name
	c.mu.Unlock()
}

func (c *Client) cachedName(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.names[id]
}

func (c *Client) nameOf(ctx context.Context, accessToken, id string) string {
	if name := c.cachedName(id); name != "" {
		return name
	}
	item, _, err := c.detail(ctx, accessToken, id)
	if err != nil {
		return ""
	}
	c.rememberName(id, item.Name)
	return item.Name
}

func (c *Client) pathFor(ctx context.Context, accessToken, directoryID string, entries []fileItem) []pan.Directory {
	if directoryID == "" || directoryID == "0" {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		if entry.FullParentID != "" {
			ids = splitIDs(entry.FullParentID)
			break
		}
	}
	if len(ids) == 0 {
		item, _, err := c.detail(ctx, accessToken, directoryID)
		if err == nil {
			ids = splitIDs(item.FullParentID)
			if item.FileID != "" {
				ids = append(ids, item.FileID)
			}
			c.rememberName(item.FileID, item.Name)
		}
	}
	if len(ids) == 0 || ids[len(ids)-1] != directoryID {
		ids = append(ids, directoryID)
	}
	return c.pathFromIDs(ctx, accessToken, ids)
}

func (c *Client) pathFromIDs(ctx context.Context, accessToken string, ids []string) []pan.Directory {
	result := make([]pan.Directory, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		result = append(result, pan.Directory{ID: id, Name: c.nameOf(ctx, accessToken, id)})
	}
	return result
}

func splitIDs(value string) []string {
	parts := strings.Split(value, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func numberFloat(value json.Number) (float64, bool) {
	if value == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(value.String(), 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func clampProgress(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func magnetHash(uri string) (string, bool) {
	lower := strings.ToLower(uri)
	index := strings.Index(lower, "btih:")
	if index < 0 {
		return "", false
	}
	hash := lower[index+len("btih:"):]
	if end := strings.IndexAny(hash, "&?"); end >= 0 {
		hash = hash[:end]
	}
	if len(hash) != 40 || !isHex(hash) {
		return "", false
	}
	return hash, true
}

func magnetHashOfTask(task cloudTask) string {
	if hash, ok := magnetHash(task.Res); ok {
		return hash
	}
	return ""
}

func isHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
