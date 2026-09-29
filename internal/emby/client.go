package emby

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

// embyClient owns HTTP details, while callers supply a configuration snapshot.
type embyClient struct{ http *http.Client }

func newEmbyClient() *embyClient { return &embyClient{http: &http.Client{Timeout: 10 * time.Second}} }

func (c *embyClient) request(ctx context.Context, cfg Config, method, endpoint string, body io.Reader, contentType string, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(cfg.ServerURL, "/")+endpoint, body)
	if err != nil {
		return domain.E(domain.KindInvalid, "创建 Emby 请求失败", err)
	}
	req.Header.Set("X-Emby-Token", cfg.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.E(domain.KindUpstream, fmt.Sprintf("无法连接到 Emby 服务器: %v", err), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return domain.E(domain.KindUnauthorized, "Emby API Key 无效或权限不足", nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return domain.E(domain.KindUpstream, fmt.Sprintf("Emby 服务器返回错误 (HTTP %d): %s", resp.StatusCode, message), nil)
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return domain.E(domain.KindUpstream, "解析 Emby 响应失败", err)
		}
	}
	return nil
}

// ServerInfo is the application's response format, independent of Emby's wire format.
type ServerInfo struct {
	ServerName string `json:"server_name"`
	Version    string `json:"version"`
	ID         string `json:"id"`
}

func (c *embyClient) ping(ctx context.Context, cfg Config) (ServerInfo, error) {
	var raw struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
		ID         string `json:"Id"`
	}
	if err := c.request(ctx, cfg, http.MethodGet, "/System/Info", nil, "", &raw); err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{ServerName: raw.ServerName, Version: raw.Version, ID: raw.ID}, nil
}

type mediaUpdateItem struct {
	Path       string `json:"Path"`
	UpdateType string `json:"UpdateType"`
}
type mediaUpdateRequest struct {
	Updates []mediaUpdateItem `json:"Updates"`
}

func (c *embyClient) notify(ctx context.Context, cfg Config, updates []mediaUpdateItem) error {
	body, err := json.Marshal(mediaUpdateRequest{Updates: updates})
	if err != nil {
		return err
	}
	return c.request(ctx, cfg, http.MethodPost, "/Library/Media/Updated", bytes.NewReader(body), "application/json", nil)
}

func (c *embyClient) refresh(ctx context.Context, cfg Config) error {
	return c.request(ctx, cfg, http.MethodPost, "/Library/Refresh", nil, "", nil)
}

type PersonItem struct {
	Name            string            `json:"Name"`
	ID              string            `json:"Id"`
	PrimaryImageTag string            `json:"PrimaryImageTag,omitempty"`
	ImageTags       map[string]string `json:"ImageTags,omitempty"`
}

func (c *embyClient) personsWithoutAvatar(ctx context.Context, cfg Config) ([]PersonItem, error) {
	var result struct {
		Items []PersonItem `json:"Items"`
	}
	if err := c.request(ctx, cfg, http.MethodGet, "/Persons", nil, "", &result); err != nil {
		return nil, err
	}
	var missing []PersonItem
	for _, person := range result.Items {
		if person.ID != "" && strings.TrimSpace(person.Name) != "" && person.PrimaryImageTag == "" && person.ImageTags["Primary"] == "" {
			missing = append(missing, person)
		}
	}
	return missing, nil
}

func (c *embyClient) uploadAvatar(ctx context.Context, cfg Config, personID string, image domain.Media) error {
	if personID == "" || len(image.Body) == 0 {
		return nil
	}
	body := strings.NewReader(base64.StdEncoding.EncodeToString(image.Body))
	return c.request(ctx, cfg, http.MethodPost, "/Items/"+url.PathEscape(personID)+"/Images/Primary", body, image.ContentType, nil)
}
