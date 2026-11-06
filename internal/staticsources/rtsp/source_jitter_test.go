package rtsp

import (
	"encoding/json"
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
)

func TestNormalizedJitterUs(t *testing.T) {
	video := &format.H264{}
	audio := &format.Opus{}
	stats := gortsplib.SessionStats{
		Medias: map[*description.Media]gortsplib.SessionStatsMedia{
			{}: {Formats: map[format.Format]gortsplib.SessionStatsFormat{
				video: {InboundRTPPackets: 100, InboundRTPPacketsJitter: 900},
				audio: {InboundRTPPackets: 100, InboundRTPPacketsJitter: 960},
			}},
		},
	}
	jitter := normalizedJitterUs(stats)
	require.NotNil(t, jitter)
	// 900 / 90000 = 10ms; 960 / 48000 = 20ms; mean = 15ms.
	require.InDelta(t, 15_000, *jitter, 0.001)
	payload, err := json.Marshal(defs.APIStaticSourceTypeSpecificRTSP{InboundRTPPacketsJitterUs: jitter})
	require.NoError(t, err)
	require.Contains(t, string(payload), `"inboundRTPPacketsJitterUs":15000`)
}

func TestNormalizedJitterUsNoTraffic(t *testing.T) {
	stats := gortsplib.SessionStats{
		Medias: map[*description.Media]gortsplib.SessionStatsMedia{
			{}: {Formats: map[format.Format]gortsplib.SessionStatsFormat{
				&format.H264{}: {InboundRTPPacketsJitter: 90_000},
			}},
		},
	}
	require.Nil(t, normalizedJitterUs(stats))
	payload, err := json.Marshal(defs.APIStaticSourceTypeSpecificRTSP{})
	require.NoError(t, err)
	require.Contains(t, string(payload), `"inboundRTPPacketsJitterUs":null`)
}

func TestNormalizedJitterUsUnknownClock(t *testing.T) {
	stats := gortsplib.SessionStats{
		Medias: map[*description.Media]gortsplib.SessionStatsMedia{
			{}: {Formats: map[format.Format]gortsplib.SessionStatsFormat{
				&format.Generic{}: {InboundRTPPackets: 100, InboundRTPPacketsJitter: 100},
			}},
		},
	}
	require.Nil(t, normalizedJitterUs(stats))
}
