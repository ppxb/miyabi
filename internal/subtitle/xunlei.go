package subtitle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/ppxb/miyabi/internal/netx"
)

const defaultXunleiEndpoint = "https://api-shoulei-ssl.xunlei.com/oracle/subtitle"

type XunleiProvider struct {
	endpoint string
	client   *resty.Client
}

func NewXunleiProvider(proxyManager *netx.ProxyManager) *XunleiProvider {
	var client *resty.Client
	opts := netx.RestyOptions{Timeout: 10 * time.Second}
	if proxyManager != nil {
		client = netx.NewRestyClient(proxyManager, opts)
	} else {
		client = netx.NewDirectRestyClient(opts)
	}
	return &XunleiProvider{endpoint: defaultXunleiEndpoint, client: client}
}

func (p *XunleiProvider) Name() string {
	return "迅雷"
}

type xunleiResponse struct {
	Code int          `json:"code"`
	Data []xunleiItem `json:"data"`
}

type xunleiItem struct {
	Name      string   `json:"name"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Ext       string   `json:"ext"`
	Languages []string `json:"languages"`
}

func (p *XunleiProvider) Search(ctx context.Context, code string) ([]Candidate, error) {
	clean := strings.TrimSpace(code)
	if clean == "" {
		return nil, nil
	}

	reqURL := fmt.Sprintf("%s?name=%s", p.endpoint, url.QueryEscape(clean))
	resp, err := p.client.R().SetContext(ctx).Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("xunlei request: %w", err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("xunlei status %d", resp.StatusCode())
	}

	var parsed xunleiResponse
	if err := json.Unmarshal(resp.Body(), &parsed); err != nil {
		return nil, fmt.Errorf("decode xunlei json: %w", err)
	}

	if parsed.Code != 0 || len(parsed.Data) == 0 {
		return nil, nil
	}

	candidates := make([]Candidate, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.URL == "" {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.Title
		}
		ext := item.Ext
		if ext == "" {
			ext = "srt"
		}
		candidates = append(candidates, Candidate{
			Provider: p.Name(),
			Name:     name,
			URL:      item.URL,
			Format:   Format(ext),
			Language: LanguageHint(strings.Join(item.Languages, " ") + " " + name),
			Version:  DetectVersion(name),
		})
	}

	return candidates, nil
}
