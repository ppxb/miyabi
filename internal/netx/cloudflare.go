package netx

import "bytes"

// IsCloudflareInterstitial reports whether body is a real Cloudflare challenge
// page. It ignores the "challenge-platform" detection script URL that normal
// Cloudflare-fronted pages reference, avoiding false positives.
func IsCloudflareInterstitial(body []byte) bool {
	lower := bytes.ToLower(body)
	return bytes.Contains(lower, []byte("just a moment")) ||
		bytes.Contains(lower, []byte("attention required! | cloudflare")) ||
		bytes.Contains(lower, []byte("cf-challenge")) ||
		bytes.Contains(lower, []byte("_cf_chl_"))
}
