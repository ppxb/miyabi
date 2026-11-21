package netx

import "bytes"

// IsCloudflareChallenge reports whether body is a Cloudflare interstitial /
// challenge page rather than the real response.
func IsCloudflareChallenge(body []byte) bool {
	return bytes.Contains(body, []byte("Just a moment...")) ||
		bytes.Contains(body, []byte("Attention Required! | Cloudflare")) ||
		bytes.Contains(body, []byte("challenge-platform"))
}

// IsCloudflareInterstitial reports a real Cloudflare challenge page. Unlike
// IsCloudflareChallenge it ignores the "challenge-platform" detection script URL
// that normal Cloudflare-fronted pages reference, avoiding false positives.
func IsCloudflareInterstitial(body []byte) bool {
	lower := bytes.ToLower(body)
	return bytes.Contains(lower, []byte("just a moment")) ||
		bytes.Contains(lower, []byte("attention required! | cloudflare")) ||
		bytes.Contains(lower, []byte("cf-challenge")) ||
		bytes.Contains(lower, []byte("_cf_chl_"))
}
