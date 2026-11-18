package playback

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const maxPlaylistSize = 2 << 20

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
