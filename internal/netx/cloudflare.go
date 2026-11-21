package netx

import "bytes"

// IsCloudflareChallenge reports whether body is a Cloudflare interstitial /
// challenge page rather than the real response.
func IsCloudflareChallenge(body []byte) bool {
	return bytes.Contains(body, []byte("Just a moment...")) ||
		bytes.Contains(body, []byte("Attention Required! | Cloudflare")) ||
		bytes.Contains(body, []byte("challenge-platform"))
}
