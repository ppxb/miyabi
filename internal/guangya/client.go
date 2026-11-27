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

func (c *Client) call(ctx context.Context, method, address string, headers map[string]string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.E(domain.KindUpstream, "光鸭请求失败", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return domain.E(domain.KindUpstream, "读取光鸭响应失败", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return pan.ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.E(domain.KindUpstream,
			fmt.Sprintf("光鸭返回 HTTP %d", resp.StatusCode), nil)
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

func (c *Client) callAccount(ctx context.Context, method, token, path string, body any, out any, extra map[string]string) error {
	headers := c.accountHeaders()
	if token != "" {
		headers["authorization"] = "Bearer " + token
	}
	for key, value := range extra {
		headers[key] = value
	}
	return c.call(ctx, method, c.accountBase+path, headers, body, out)
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

// Account returns the signed-in profile. Guangya has no avatar/space in this
// endpoint; the settings card still renders with zeroed values.
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
	return pan.Account{ID: me.Sub, Name: name}, nil
}

func (c *Client) BeginLogin(context.Context) (*pan.Login, error) {
	return nil, domain.E(domain.KindInvalid, "光鸭使用短信登录", nil)
}

func (c *Client) LoginStatus(context.Context, *pan.Login) (pan.LoginState, error) {
	return pan.LoginExpired, domain.E(domain.KindInvalid, "光鸭使用短信登录", nil)
}

func (c *Client) ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error) {
	return pan.Tokens{}, domain.E(domain.KindInvalid, "光鸭使用短信登录", nil)
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

// SMSInit starts the SMS login and returns the captcha token required by the
// send step. If the upstream asks for a human challenge, an error is returned.
func (c *Client) SMSInit(ctx context.Context, phone string) (string, error) {
	var result struct {
		CaptchaToken string `json:"captcha_token"`
		URL          string `json:"url"`
	}
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/shield/captcha/init", map[string]any{
		"client_id": clientID,
		"action":    "POST:/v1/auth/verification",
		"device_id": c.did,
		"meta":      map[string]string{"phone_number": phone},
	}, &result, nil)
	if err != nil {
		return "", err
	}
	if result.CaptchaToken == "" {
		if result.URL != "" {
			return "", domain.E(domain.KindConflict, "光鸭要求人机验证，请在浏览器完成后再试", nil)
		}
		return "", domain.E(domain.KindUpstream, "光鸭未返回验证码凭证", nil)
	}
	return result.CaptchaToken, nil
}

func (c *Client) SMSSend(ctx context.Context, phone, captchaToken string) (string, error) {
	var result struct {
		VerificationID string `json:"verification_id"`
	}
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/auth/verification", map[string]string{
		"phone_number": phone,
		"target":       "ANY",
		"client_id":    clientID,
	}, &result, map[string]string{"x-captcha-token": captchaToken})
	if err != nil {
		return "", err
	}
	if result.VerificationID == "" {
		return "", domain.E(domain.KindUpstream, "光鸭未返回验证码 ID", nil)
	}
	return result.VerificationID, nil
}

func (c *Client) SMSVerify(ctx context.Context, verificationID, code string) (string, error) {
	var result struct {
		VerificationToken string `json:"verification_token"`
	}
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/auth/verification/verify", map[string]string{
		"verification_id":   verificationID,
		"verification_code": code,
		"client_id":         clientID,
	}, &result, nil)
	if err != nil {
		return "", err
	}
	if result.VerificationToken == "" {
		return "", domain.E(domain.KindInvalid, "验证码校验失败", nil)
	}
	return result.VerificationToken, nil
}

func (c *Client) SignIn(ctx context.Context, phone, code, verificationToken, captchaToken string) (pan.Tokens, error) {
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	err := c.callAccount(ctx, http.MethodPost, "", "/v1/auth/signin", map[string]string{
		"verification_code":  code,
		"verification_token": verificationToken,
		"username":           phone,
		"client_id":          clientID,
	}, &result, map[string]string{"x-captcha-token": captchaToken})
	if err != nil {
		return pan.Tokens{}, err
	}
	if result.AccessToken == "" {
		return pan.Tokens{}, pan.ErrUnauthorized
	}
	return tokensOf(result.AccessToken, result.RefreshToken, result.ExpiresIn), nil
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

func (c *Client) detail(ctx context.Context, accessToken, fileID string) (fileItem, string, error) {
	var data struct {
		FileInfo      fileItem `json:"fileInfo"`
		VideoResource []struct {
			GCID string `json:"gcid"`
		} `json:"videoResource"`
	}
	err := c.callAPI(ctx, accessToken, "/nd.bizuserres.s/v1/file/get_file_detail",
		map[string]string{"fileId": fileID}, &data)
	if err != nil {
		return fileItem{}, "", err
	}
	var gcid string
	if len(data.VideoResource) > 0 {
		gcid = data.VideoResource[0].GCID
	}
	return data.FileInfo, gcid, nil
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
	err := c.callAPI(ctx, accessToken, "/nd.bizuserres.s/v1/file/get_vod_download_url",
		map[string]string{"fileId": fileID, "gcid": gcid}, &data)
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
