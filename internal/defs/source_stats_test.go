package defs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// compile-time assertions that every concrete type satisfies the marker.
var (
	_ StaticSourceStats = (*RTSPSourceStats)(nil)
	_ StaticSourceStats = (*RTPSourceStats)(nil)
	_ StaticSourceStats = (*WebRTCSourceStats)(nil)
	_ StaticSourceStats = (*SRTSourceStats)(nil)
)

func TestBaseSourceStatsJitterOmittedWhenNil(t *testing.T) {
	stats := &SRTSourceStats{
		BaseSourceStats: BaseSourceStats{
			PacketsReceived: 10,
			PacketsLost:     2,
			// Jitter nil
		},
	}

	byts, err := json.Marshal(stats)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(byts, &m))

	_, ok := m["inboundRTPPacketsJitter"]
	require.False(t, ok, "jitter key must be omitted when nil")

	require.Equal(t, float64(10), m["inboundRTPPackets"])
	require.Equal(t, float64(2), m["inboundRTPPacketsLost"])
}

func TestBaseSourceStatsJitterPresentWhenSet(t *testing.T) {
	jitter := 0.0125
	stats := &WebRTCSourceStats{
		BaseSourceStats: BaseSourceStats{
			PacketsReceived: 100,
			PacketsLost:     5,
			Jitter:          &jitter,
		},
	}

	byts, err := json.Marshal(stats)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(byts, &m))

	v, ok := m["inboundRTPPacketsJitter"]
	require.True(t, ok, "jitter key must be present when set")
	require.Equal(t, 0.0125, v)
}

func TestRTSPSourceStatsJSON(t *testing.T) {
	jitter := 0.5
	stats := &RTSPSourceStats{
		BaseSourceStats: BaseSourceStats{
			PacketsReceived: 1000,
			PacketsLost:     7,
			Jitter:          &jitter,
		},
		PacketsInError: 3,
	}

	byts, err := json.Marshal(stats)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(byts, &m))

	require.Equal(t, float64(1000), m["inboundRTPPackets"])
	require.Equal(t, float64(7), m["inboundRTPPacketsLost"])
	require.Equal(t, 0.5, m["inboundRTPPacketsJitter"])
	require.Equal(t, float64(3), m["inboundRTPPacketsInError"])
}

func TestRTPSourceStatsJSON(t *testing.T) {
	stats := &RTPSourceStats{
		BaseSourceStats: BaseSourceStats{
			PacketsReceived: 42,
			PacketsLost:     1,
			// Jitter nil for raw RTP
		},
	}

	byts, err := json.Marshal(stats)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(byts, &m))

	require.Equal(t, float64(42), m["inboundRTPPackets"])
	require.Equal(t, float64(1), m["inboundRTPPacketsLost"])
	_, ok := m["inboundRTPPacketsJitter"]
	require.False(t, ok, "raw RTP must not report jitter")
}

// StaticStats round-trips through the SourceStats marker interface
// as it does inside APIPath.
func TestStaticSourceStatsViaInterface(t *testing.T) {
	var s StaticSourceStats = &SRTSourceStats{
		BaseSourceStats: BaseSourceStats{PacketsReceived: 5, PacketsLost: 0},
	}

	byts, err := json.Marshal(s)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(byts, &m))
	require.Equal(t, float64(5), m["inboundRTPPackets"])
}
