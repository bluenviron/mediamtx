package webrtc

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionHostsToAdvertise(t *testing.T) {
	for _, ca := range []struct {
		name             string
		ipFromHostHeader bool
		additionalHosts  []string
		host             string
		expected         []string
	}{
		{
			name:             "disabled",
			ipFromHostHeader: false,
			additionalHosts:  []string{"1.2.3.4"},
			host:             "example.com:8889",
			expected:         []string{"1.2.3.4"},
		},
		{
			name:             "enabled, IP host with port",
			ipFromHostHeader: true,
			additionalHosts:  []string{"1.2.3.4"},
			host:             "203.0.113.10:8889",
			expected:         []string{"1.2.3.4", "203.0.113.10"},
		},
		{
			name:             "enabled, IP host without port",
			ipFromHostHeader: true,
			additionalHosts:  []string{},
			host:             "203.0.113.11",
			expected:         []string{"203.0.113.11"},
		},
		{
			name:             "enabled, empty host",
			ipFromHostHeader: true,
			additionalHosts:  []string{"1.2.3.4"},
			host:             "",
			expected:         []string{"1.2.3.4"},
		},
		{
			// the Host header is client-controlled and unauthenticated by
			// default; a non-IP value must never be forwarded to the DNS
			// resolver in addAdditionalCandidates(), so it's dropped rather
			// than advertised.
			name:             "enabled, non-IP host is rejected",
			ipFromHostHeader: true,
			additionalHosts:  []string{"1.2.3.4"},
			host:             "example.com:8889",
			expected:         []string{"1.2.3.4"},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			s := &session{
				additionalHosts:  ca.additionalHosts,
				ipFromHostHeader: ca.ipFromHostHeader,
				httpRequest:      &http.Request{Host: ca.host},
			}

			require.Equal(t, ca.expected, s.hostsToAdvertise())
		})
	}
}
