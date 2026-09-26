package netx

import (
	"net"
	"testing"
)

func TestOutboundIP(t *testing.T) {
	ip := OutboundIP()
	if ip == "" {
		t.Fatal("OutboundIP returned empty string")
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("OutboundIP returned invalid IP: %s", ip)
	}
}
