package pan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const mediaUserAgent = "Miyabi/1.0"

type PlaySource struct {
	URL        string `json:"url"`
	Height     int    `json:"height"`
	Definition int    `json:"definition"`
}

func (client *Client) DownloadURL(ctx context.Context, accessToken, pickCode string) (string, error) {
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken).
		SetHeader("User-Agent", mediaUserAgent).SetFormData(map[string]string{"pick_code": pickCode}),
		http.MethodPost, apiURL+"/open/ufile/downurl")
	if err != nil {
		return "", err
	}
	var result struct {
		apiResponse
		Data map[string]struct {
			URL struct {
				URL string `json:"url"`
			} `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return "", fmt.Errorf("decode 115 download URL: %w", err)
	}
	if err := result.err(); err != nil {
		return "", err
	}
	if len(result.Data) != 1 {
		return "", fmt.Errorf("115 returned no unique download URL")
	}
	var address string
	for _, item := range result.Data {
		address = item.URL.URL
	}
	if address == "" {
		return "", fmt.Errorf("115 download URL is empty")
	}
	return address, nil
}

func (client *Client) PlayURL(ctx context.Context, accessToken, pickCode string, hls bool) ([]PlaySource, error) {
	if !hls {
		address, err := client.DownloadURL(ctx, accessToken, pickCode)
		if err != nil {
			return nil, err
		}
		return []PlaySource{{URL: address}}, nil
	}
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken).
		SetHeader("User-Agent", mediaUserAgent).SetQueryParam("pick_code", pickCode),
		http.MethodGet, apiURL+"/open/video/play")
	if err != nil {
		return nil, err
	}
	var result struct {
		apiResponse
		Data struct {
			Sources []PlaySource `json:"video_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, fmt.Errorf("decode 115 playback URLs: %w", err)
	}
	if err := result.err(); err != nil {
		return nil, err
	}
	if len(result.Data.Sources) == 0 {
		return nil, fmt.Errorf("115 尚未提供该文件的转码播放地址，请使用原文件播放或稍后重试")
	}
	for _, source := range result.Data.Sources {
		if source.URL == "" || source.Height <= 0 {
			return nil, fmt.Errorf("115 returned incomplete playback source")
		}
	}
	return result.Data.Sources, nil
}

// OpenMedia streams CDN responses without the API rate limiter or OAuth headers.
// The caller owns the body, including for unsuccessful HTTP responses.
func (client *Client) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	request := client.media.R().SetContext(ctx).SetDoNotParseResponse(true).
		SetHeader("User-Agent", mediaUserAgent).SetHeader("Accept-Encoding", "identity")
	for _, name := range []string{"Range", "If-Range"} {
		if value := headers.Get(name); value != "" {
			request.SetHeader(name, value)
		}
	}
	response, err := request.Execute(method, address)
	if err != nil {
		if response != nil && response.RawBody() != nil {
			response.RawBody().Close()
		}
		// net/http includes signed CDN URLs in url.Error; keep only its underlying cause.
		var requestError *url.Error
		if errors.As(err, &requestError) {
			err = requestError.Err
		}
		return nil, fmt.Errorf("request 115 media: %w", err)
	}
	return response.RawResponse, nil
}
