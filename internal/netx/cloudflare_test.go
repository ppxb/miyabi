package netx

import "testing"

func TestIsCloudflareInterstitialIgnoresDetectionScript(t *testing.T) {
	normal := []byte(`<html><script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js?ray=abc"></script><body>ok</body></html>`)
	if IsCloudflareInterstitial(normal) {
		t.Fatal("normal page referencing the challenge-platform script must not be a challenge")
	}
}

func TestIsCloudflareInterstitialDetectsRealChallenges(t *testing.T) {
	for _, body := range []string{
		`<title>Just a moment...</title>`,
		`<title>Attention Required! | Cloudflare</title>`,
		`<script src="/cdn-cgi/challenge-platform/h/b/cf-challenge"></script>`,
		`window._cf_chl_opt = {}`,
	} {
		if !IsCloudflareInterstitial([]byte(body)) {
			t.Errorf("expected challenge for %q", body)
		}
	}
}
