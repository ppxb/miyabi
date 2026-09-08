package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const maxPlaylistSize = 2 << 20

type playBody struct {
	io.Reader
	io.Closer
	release func()
}

func (body *playBody) Close() error {
	body.release()
	return body.Closer.Close()
}

func (service *PlayService) Stream(ctx context.Context, id string, index int, method string, headers http.Header) (*http.Response, error) {
	session, resource, err := service.resource(id, index)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(session.ctx, cancel)
	release := func() { stop(); cancel() }
	if resource.playlist {
		// A rewritten playlist is a new representation and cannot reuse upstream byte ranges.
		headers = nil
	}
	response, err := service.library.drive.client.OpenMedia(ctx, method, resource.url.String(), headers)
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
			return nil, fmt.Errorf("播放地址已失效，请重新加载播放")
		}
		return nil, fmt.Errorf("115 视频流返回 HTTP %d", response.StatusCode)
	}
	if method == http.MethodHead || response.StatusCode == http.StatusPartialContent {
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

var playlistURI = regexp.MustCompile(`([:,])URI="([^"]*)"`)

// Rewrites URI lines and URI attributes, preserving HLS tags and byte-range metadata.
func rewritePlaylist(body []byte, base *url.URL, register func(*url.URL, bool) (string, error)) (string, error) {
	rewrite := func(value string, playlist bool) (string, error) {
		reference, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("115 playlist contains an invalid URI")
		}
		return register(base.ResolveReference(reference), playlist)
	}
	lines := strings.Split(string(body), "\n")
	nextPlaylist := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			local, err := rewrite(trimmed, nextPlaylist)
			if err != nil {
				return "", err
			}
			lines[i] = local
			nextPlaylist = false
			continue
		}
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF:") {
			nextPlaylist = true
		}
		playlist := strings.HasPrefix(trimmed, "#EXT-X-MEDIA:") ||
			strings.HasPrefix(trimmed, "#EXT-X-I-FRAME-STREAM-INF:") ||
			strings.HasPrefix(trimmed, "#EXT-X-IMAGE-STREAM-INF:") ||
			strings.HasPrefix(trimmed, "#EXT-X-RENDITION-REPORT:")
		var rewriteError error
		lines[i] = playlistURI.ReplaceAllStringFunc(line, func(attribute string) string {
			match := playlistURI.FindStringSubmatch(attribute)
			local, err := rewrite(match[2], playlist)
			if err != nil {
				rewriteError = err
				return ""
			}
			return match[1] + `URI="` + local + `"`
		})
		if rewriteError != nil {
			return "", rewriteError
		}
	}
	return strings.Join(lines, "\n"), nil
}
