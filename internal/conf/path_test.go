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
	for _, ca := range []struct {
		name   string
		source string
		rtpSDP string
	}{
		{
			name:   "rtmp_with_placeholders",
			source: "rtmp://$G1:$G2/live?token=$G3",
		},
		{
			name:   "https_with_placeholders",
			source: "https://$G1/$G2/index.m3u8",
		},
		{
			name:   "srt_with_placeholders",
			source: "srt://$G1:$G2/$G3",
		},
		{
			name:   "whep_with_placeholders",
			source: "whep://$G1:$G2/$G3",
		},
		{
			name:   "wheps_with_placeholders",
			source: "wheps://$G1:$G2/$G3",
		},
		{
			name:   "udp_with_placeholders",
			source: "udp://$G1:$G2",
		},
		{
			name:   "udp_mpegts_with_placeholders",
			source: "udp+mpegts://$G1:$G2",
		},
		{
			name:   "udp_rtp_with_placeholders",
			source: "udp+rtp://$G1:$G2",
			rtpSDP: "v=0...",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			pconf := &Path{
				Source:     ca.source,
				RTPSDP:     ca.rtpSDP,
				Name:       "test",
				RecordPath: "/tmp/%path/%s",
			}
			err := pconf.validate(&Conf{}, "test", false, testNilLogger)
			require.NoError(t, err)
		})
	}
}

func TestPathValidateRejectsInvalidPlaceholderSources(t *testing.T) {
	for _, ca := range []struct {
		name   string
		source string
	}{
		{
			name:   "rtmp_missing_authority",
			source: "rtmp://$G1:$G2@",
		},
		{
			name:   "udp_missing_port",
			source: "udp://$G1",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			pconf := &Path{
				Source:     ca.source,
				Name:       "test",
				RecordPath: "/tmp/%path/%s",
			}
			err := pconf.validate(&Conf{}, "test", false, testNilLogger)
			require.Error(t, err)
		})
	}
}
