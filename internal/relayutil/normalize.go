package relayutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func NormalizeRelayURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	if !strings.HasPrefix(rawURL, "ws://") && !strings.HasPrefix(rawURL, "wss://") {
		rawURL = "wss://" + rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme != "ws" && parsedURL.Scheme != "wss" {
		return "", fmt.Errorf("invalid scheme: %s (must be ws or wss)", parsedURL.Scheme)
	}

	host := strings.ToLower(parsedURL.Hostname())

	if strings.HasSuffix(host, ".onion") {
		return "", fmt.Errorf("Tor relays not supported")
	}

	if isLocalhost(host) {
		return "", fmt.Errorf("localhost not supported")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return "", fmt.Errorf("private IP addresses not supported")
		}
	}

	port := parsedURL.Port()
	if port == "" {
		if parsedURL.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}

	normalized := parsedURL.Scheme + "://" + host
	if (parsedURL.Scheme == "wss" && port != "443") || (parsedURL.Scheme == "ws" && port != "80") {
		normalized += ":" + port
	}

	return normalized, nil
}

func isLocalhost(host string) bool {
	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		host == "0.0.0.0"
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
	}

	for _, block := range privateBlocks {
		_, subnet, _ := net.ParseCIDR(block)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}
