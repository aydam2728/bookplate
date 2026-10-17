package metadata

import (
	"errors"
	"testing"
)

func TestBlockPrivateAddresses(t *testing.T) {
	tests := []struct {
		address string
		blocked bool
		why     string
	}{
		{"93.184.216.34:443", false, "an ordinary public address"},
		{"127.0.0.1:443", true, "loopback: the instance itself"},
		{"[::1]:443", true, "loopback over IPv6"},
		{"10.0.0.5:443", true, "private range: the home network"},
		{"192.168.1.1:443", true, "private range: the router"},
		{"172.16.0.1:443", true, "private range"},
		{"169.254.169.254:443", true, "cloud metadata, the classic SSRF target"},
		{"[fe80::1]:443", true, "link-local over IPv6"},
		{"0.0.0.0:443", true, "unspecified"},
		{"not-an-address", true, "unparseable"},
	}

	for _, tt := range tests {
		t.Run(tt.why, func(t *testing.T) {
			err := blockPrivateAddresses("tcp4", tt.address, nil)
			if tt.blocked && !errors.Is(err, ErrBlocked) {
				t.Errorf("%s was allowed (%s), err = %v", tt.address, tt.why, err)
			}
			if !tt.blocked && err != nil {
				t.Errorf("%s was refused (%s): %v", tt.address, tt.why, err)
			}
		})
	}
}

func TestBlockPrivateAddresses_RefusesUnixSockets(t *testing.T) {
	if err := blockPrivateAddresses("unix", "/var/run/docker.sock", nil); !errors.Is(err, ErrBlocked) {
		t.Errorf("a unix socket was allowed, err = %v", err)
	}
}
