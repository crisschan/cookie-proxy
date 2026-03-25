package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// CheckURL returns an error if the URL is unsafe (SSRF targets).
func CheckURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https schemes are allowed")
	}

	host := u.Hostname()

	// Block obvious hostnames
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("SSRF: private hostname blocked")
	}

	// Resolve and check IP ranges
	addrs, err := net.LookupHost(host)
	if err != nil {
		// If we can't resolve, treat the host itself as an IP
		addrs = []string{host}
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("SSRF: private IP blocked")
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	private := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
