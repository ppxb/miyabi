package netx

import (
	"net"
)

// OutboundIP first uses the IPv4 address selected by the system's outbound route.
// If unavailable, it checks interfaces, preferring private addresses over other
// usable IPv4 addresses, then falls back to "127.0.0.1". The result may be public.
func OutboundIP() string {
	conn, err := net.Dial("udp", "223.5.5.5:80")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && localAddr.IP != nil {
			ip := localAddr.IP.To4()
			if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip.String()
			}
		}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}
		if ip4.IsPrivate() {
			return ip4.String()
		}
		if fallback == "" {
			fallback = ip4.String()
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}
