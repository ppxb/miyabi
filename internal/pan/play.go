package pan

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const mediaUserAgent = "Miyabi/1.0"

type PlaySource struct {
	URL        string `json:"url"`
	Height     int    `json:"height"`
	Definition int    `json:"definition"`
}

func (client *Client) DownloadURL(ctx context.Context, accessToken, pickCode, userAgent string) (string, error) {
	type downloadURLWire struct {
		apiResponse
		Data map[string]struct {
			URL struct {
				URL string `json:"url"`
			} `json:"url"`
		} `json:"data"`
	}
	ua := strings.TrimSpace(userAgent)
	req := client.http.R().SetContext(ctx).SetAuthToken(accessToken).
		SetFormData(map[string]string{"pick_code": pickCode})
	if ua != "" {
		req.SetHeader("User-Agent", ua)
	} else {
		req.SetHeader("User-Agent", "__EMPTY__")
	}
	result, err := apiRequest[downloadURLWire](
		client,
		req,
		http.MethodPost,
		apiURL+"/open/ufile/downurl",
		"download URL",
	)
	if err != nil {
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

func (client *Client) PlayURL(ctx context.Context, accessToken, pickCode string) ([]PlaySource, error) {
	type playURLWire struct {
		apiResponse
		Data struct {
			Sources []PlaySource `json:"video_url"`
		} `json:"data"`
	}
	result, err := apiRequest[playURLWire](
		client,
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).
			SetHeader("User-Agent", mediaUserAgent).SetQueryParam("pick_code", pickCode),
		http.MethodGet,
		apiURL+"/open/video/play",
		"playback URLs",
	)
	if err != nil {
		return nil, err
	}
	if len(result.Data.Sources) == 0 {
		return nil, ErrTranscodeUnavailable
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
	ua := headers.Get("User-Agent")
	if ua == "" {
		ua = mediaUserAgent
	}
	request := client.media.R().SetContext(ctx).SetDoNotParseResponse(true).
		SetHeader("User-Agent", ua).SetHeader("Accept-Encoding", "identity")
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
