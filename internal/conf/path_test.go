package conf

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/stretchr/testify/require"
)

type localNilLogger struct{}

func (localNilLogger) Log(_ logger.Level, _ string, _ ...any) {}

var testNilLogger logger.Writer = &localNilLogger{}

func TestPathClone(t *testing.T) {
	original := &Path{
		Name:                "example",
		RTSPTransport:       RTSPTransport{new(gortsplib.ProtocolUDP)},
		SourceAnyPortEnable: new(true),
		RecordPath:          "/var/recordings",
	}

	clone := original.Clone()
	require.Equal(t, original, clone)
}

func TestIsValidPathName(t *testing.T) {
	for _, ca := range []struct {
		name   string
		path   string
		errMsg string
	}{
		{
			name: "valid nested path",
			path: "group/cam1",
		},
		{
			name: "valid dots inside segment",
			path: "cam.v1/main",
		},
		{
			name:   "parent directory",
			path:   "../cam1",
			errMsg: "can't contain dot path segments",
		},
		{
			name:   "embedded parent directory",
			path:   "group/../cam1",
			errMsg: "can't contain dot path segments",
		},
		{
			name:   "current directory",
			path:   "./cam1",
			errMsg: "can't contain dot path segments",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			err := IsValidPathName(ca.path)
			if ca.errMsg != "" {
				require.EqualError(t, err, ca.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPathValidateAllowsPlaceholdersForAllSourceProtocols(t *testing.T) {
	testCases := []struct {
		name   string
		source string
		rtpSDP string
	}{
		{
			name:   "rtmp_with_placeholders",
			source: "rtmp://$1:$2/live?token=$3",
		},
		{
			name:   "https_with_placeholders",
			source: "https://$1/$2/index.m3u8",
		},
		{
			name:   "srt_with_placeholders",
			source: "srt://$1:$2/$3",
		},
		{
			name:   "whep_with_placeholders",
			source: "whep://$1:$2/$3",
		},
		{
			name:   "wheps_with_placeholders",
			source: "wheps://$1:$2/$3",
		},
		{
			name:   "udp_with_placeholders",
			source: "udp://$1:$2",
		},
		{
			name:   "udp_mpegts_with_placeholders",
			source: "udp+mpegts://$1:$2",
		},
		{
			name:   "udp_rtp_with_placeholders",
			source: "udp+rtp://$1:$2",
			rtpSDP: "v=0...",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pconf := &Path{
				Source:     tc.source,
				RTPSDP:     tc.rtpSDP,
				Name:       "test",
				RecordPath: "/tmp/%path/%s",
			}
			err := pconf.validate(&Conf{}, "test", false, testNilLogger)
			require.NoError(t, err)
		})
	}
}
