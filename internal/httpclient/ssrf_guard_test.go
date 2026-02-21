package httpclient

import (
	"net"
	"testing"
)

func TestSSRFGuard_ValidateURL(t *testing.T) {
	guard := NewSSRFGuard()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Valid URLs
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"valid with path", "https://example.com/page/subpage", false},
		{"valid with query", "https://example.com?foo=bar", false},
		{"valid with port", "https://example.com:8443/page", false},

		// Invalid schemes
		{"ftp scheme", "ftp://example.com/", true},
		{"file scheme", "file:///etc/passwd", true},
		{"data scheme", "data:text/html,<script>alert(1)</script>", true},
		{"javascript scheme", "javascript:alert(1)", true},

		// Loopback addresses
		{"localhost", "http://localhost/", true},
		{"localhost with port", "http://localhost:8080/", true},
		{"127.0.0.1", "http://127.0.0.1/", true},
		{"127.0.0.1 with port", "http://127.0.0.1:80/", true},
		{"127.0.0.2", "http://127.0.0.2/", true},
		{"ipv6 loopback", "http://[::1]/", true},

		// Private IP addresses
		{"10.x.x.x", "http://10.0.0.1/", true},
		{"10.255.255.255", "http://10.255.255.255/", true},
		{"172.16.x.x", "http://172.16.0.1/", true},
		{"172.31.x.x", "http://172.31.255.255/", true},
		{"192.168.x.x", "http://192.168.1.1/", true},
		{"192.168.0.1", "http://192.168.0.1/", true},

		// Link-local addresses
		{"link-local", "http://169.254.1.1/", true},
		{"aws metadata", "http://169.254.169.254/", true},
		{"aws metadata path", "http://169.254.169.254/latest/meta-data/", true},

		// Other special addresses
		{"broadcast", "http://255.255.255.255/", true},
		{"0.0.0.0", "http://0.0.0.0/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guard.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestSSRFGuard_ValidateIP(t *testing.T) {
	guard := NewSSRFGuard()

	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		// Valid public IPs
		{"public ipv4 8.8.8.8", "8.8.8.8", false},
		{"public ipv4 1.1.1.1", "1.1.1.1", false},
		{"public ipv4 203.0.114.1", "203.0.114.1", false},

		// Invalid - Loopback
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.0.0.2", "127.0.0.2", true},
		{"ipv6 loopback", "::1", true},

		// Invalid - Private
		{"private 10.0.0.1", "10.0.0.1", true},
		{"private 10.255.255.255", "10.255.255.255", true},
		{"private 172.16.0.1", "172.16.0.1", true},
		{"private 172.31.255.255", "172.31.255.255", true},
		{"private 192.168.0.1", "192.168.0.1", true},
		{"private 192.168.255.255", "192.168.255.255", true},

		// Invalid - Link-local
		{"link-local 169.254.1.1", "169.254.1.1", true},
		{"link-local 169.254.169.254", "169.254.169.254", true},
		{"ipv6 link-local", "fe80::1", true},

		// Invalid - Other
		{"unspecified ipv4", "0.0.0.0", true},
		{"unspecified ipv6", "::", true},
		{"broadcast", "255.255.255.255", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			err := guard.ValidateIP(ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}
