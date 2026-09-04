package javdb

import "testing"

func TestSignatureGolden(t *testing.T) {
	const want = "1784134914.lpw6vgqzsp.85b53cc0034eff62f361723615a3b8e3"
	if got := signature(1784134914); got != want {
		t.Fatalf("signature() = %q, want %q", got, want)
	}
}
