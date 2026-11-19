package playback

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

type playBody struct {
	io.Reader
	io.Closer
	release func()
}

func (body *playBody) Close() error {
	body.release()
	return body.Closer.Close()
}

func (service *Service) Stream(ctx context.Context, id string, index int, method string, headers http.Header) (*http.Response, error) {
	session, resource, err := service.resource(ctx, id, index)
	if err != nil {
		return nil, err
	}
	if !resource.playlist && !session.proxySegments {
		// Direct media: let the browser stream from the 115 CDN itself instead of
		// relaying every byte through this server. (A <video> element is not
		// subject to CORS, so this is safe for the direct mp4 path.)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{resource.url.String()}},
			Body:       http.NoBody,
		}, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(session.ctx, cancel)
	release := func() { stop(); cancel() }
	if resource.playlist {
		// A rewritten playlist is a new representation and cannot reuse upstream byte ranges.
		headers = nil
	}
	response, err := service.drive.OpenMedia(ctx, method, resource.url.String(), headers)
	if err != nil {
		release()
		return nil, err
	}
	response.Body = &playBody{Reader: response.Body, Closer: response.Body, release: release}
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		response.Body.Close()
		response.Body = http.NoBody
		response.ContentLength = 0
		response.Header.Set("Content-Length", "0")
		return response, nil
	default:
		response.Body.Close()
		if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized {
			return nil, domain.E(domain.KindConflict, "播放地址已失效，请重新加载播放", nil)
		}
		return nil, domain.E(domain.KindUpstream, "115 视频流异常，请稍后重试", fmt.Errorf("upstream returned HTTP %d", response.StatusCode))
	}
	if method == http.MethodHead || response.StatusCode == http.StatusPartialContent {
		return response, nil
	}
	if !resource.playlist {
		// HLS segment relayed byte-for-byte (same-origin): no playlist sniffing.
		return response, nil
	}

	// 115 playlist URLs need not end in .m3u8. Sniff a bounded prefix without buffering video.
	reader := bufio.NewReader(response.Body)
	prefix, err := reader.Peek(len("#EXTM3U"))
	if err != nil && err != io.EOF {
		response.Body.Close()
		return nil, fmt.Errorf("read 115 media: %w", err)
	}
	if !bytes.Equal(prefix, []byte("#EXTM3U")) {
		if resource.playlist {
			response.Body.Close()
			return nil, fmt.Errorf("115 returned an invalid HLS playlist")
		}
		response.Body = struct {
			io.Reader
			io.Closer
		}{reader, response.Body}
		return response, nil
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxPlaylistSize+1))
	response.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read 115 playlist: %w", err)
	}
	if len(body) > maxPlaylistSize {
		return nil, fmt.Errorf("115 playlist exceeds %d bytes", maxPlaylistSize)
	}
	service.mu.Lock()
	if session.ctx.Err() != nil {
		service.mu.Unlock()
		return nil, session.ctx.Err()
	}
	// Relative URIs are resolved against the final URL after CDN redirects.
	rewritten, err := rewritePlaylist(body, response.Request.URL, session.register)
	service.mu.Unlock()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(strings.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	response.Header.Set("Content-Type", "application/vnd.apple.mpegurl")
	for _, name := range []string{"Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Encoding"} {
		response.Header.Del(name)
	}
	return response, nil
}
