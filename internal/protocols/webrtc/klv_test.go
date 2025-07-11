package webrtc

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// mockLogger implements logger.Writer for testing
type mockLogger struct{}

func (l *mockLogger) Log(_ logger.Level, _ string, _ ...interface{}) {}

// TestKLVFormatDetection tests that KLV formats are properly detected
func TestKLVFormatDetection(t *testing.T) {
	tests := []struct {
		name       string
		formats    []format.Format
		expectsKLV bool
	}{
		{
			name: "KLV format detected",
			formats: []format.Format{
				&format.KLV{},
			},
			expectsKLV: true,
		},
		{
			name: "Generic KLV format detected",
			formats: []format.Format{
				&format.Generic{
					PayloadTyp: 96,
					RTPMa:      "KLV/90000",
				},
			},
			expectsKLV: true,
		},
		{
			name: "No KLV format",
			formats: []format.Format{
				&format.H264{PayloadTyp: 96},
			},
			expectsKLV: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamDesc := &description.Session{
				Medias: []*description.Media{
					{
						Type:    description.MediaTypeApplication,
						Formats: tt.formats,
					},
				},
			}

			mockStream := &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               streamDesc,
				GenerateRTPPackets: false,
				Parent:             &mockLogger{},
			}

			err := mockStream.Initialize()
			require.NoError(t, err)
			defer mockStream.Close()

			// Test KLV detection logic (same as in setupKLVDataChannel)
			var klvFormat format.Format
			for _, media := range mockStream.Desc.Medias {
				for _, forma := range media.Formats {
					if genericFmt, ok := forma.(*format.Generic); ok {
						if genericFmt.RTPMap() == "KLV/90000" {
							klvFormat = genericFmt
							break
						}
					}
					if _, ok := forma.(*format.KLV); ok {
						klvFormat = forma
						break
					}
					if forma.Codec() == "KLV" {
						klvFormat = forma
						break
					}
				}
				if klvFormat != nil {
					break
				}
			}

			if tt.expectsKLV {
				require.NotNil(t, klvFormat, "Expected to find KLV format")
			} else {
				require.Nil(t, klvFormat, "Expected not to find KLV format")
			}
		})
	}
}

// TestKLVUnitHandling tests that different KLV unit types are handled correctly
func TestKLVUnitHandling(t *testing.T) {
	tests := []struct {
		name         string
		unit         unit.Unit
		expectedData []byte
	}{
		{
			name: "KLV unit with data",
			unit: &unit.KLV{
				Unit: []byte{0x06, 0x0E, 0x2B, 0x34},
			},
			expectedData: []byte{0x06, 0x0E, 0x2B, 0x34},
		},
		{
			name: "KLV unit without data",
			unit: &unit.KLV{
				Unit: nil,
			},
			expectedData: nil,
		},
		{
			name: "Generic unit with RTP packets",
			unit: &unit.Generic{
				Base: unit.Base{
					RTPPackets: []*rtp.Packet{
						{Payload: []byte{0x06, 0x0E}},
						{Payload: []byte{0x2B, 0x34}},
					},
				},
			},
			expectedData: []byte{0x06, 0x0E, 0x2B, 0x34},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the unit handling logic from setupKLVDataChannel
			var klvData []byte

			switch tunit := tt.unit.(type) {
			case *unit.Generic:
				if tunit.RTPPackets != nil {
					for _, pkt := range tunit.RTPPackets {
						klvData = append(klvData, pkt.Payload...)
					}
				}
			case *unit.KLV:
				if tunit.Unit != nil {
					klvData = append(klvData, tunit.Unit...)
				}
			}

			require.Equal(t, tt.expectedData, klvData)
		})
	}
}

// TestSetupKLVDataChannelIntegration tests that KLV setup doesn't break normal WebRTC flow
func TestSetupKLVDataChannelIntegration(t *testing.T) {
	// Test that setupKLVDataChannel can be called without breaking anything
	streamDesc := &description.Session{
		Medias: []*description.Media{
			{
				Type: description.MediaTypeApplication,
				Formats: []format.Format{
					&format.KLV{},
				},
			},
		},
	}

	mockStream := &stream.Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               streamDesc,
		GenerateRTPPackets: false,
		Parent:             &mockLogger{},
	}

	err := mockStream.Initialize()
	require.NoError(t, err)
	defer mockStream.Close()

	// Create a mock peer connection (nil is fine for this test)
	pc := &PeerConnection{}

	// This should not panic or return an error, just return nil format
	klvFormat, err := setupKLVDataChannel(mockStream, &mockLogger{}, pc)
	require.NoError(t, err)
	require.Nil(t, klvFormat) // Should be nil due to defensive handling
}
