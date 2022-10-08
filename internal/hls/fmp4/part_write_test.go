package fmp4

import (
	"testing"
	"time"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/stretchr/testify/require"
)

func TestPartWrite(t *testing.T) {
	testVideoSamples := []*VideoSample{
		{
			NALUs: [][]byte{
				{0x06},
				{0x07},
			},
			PTS: 0,
			DTS: 0,
		},
		{
			NALUs: [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			},
			PTS: 2 * time.Second,
			DTS: 2 * time.Second,
		},

		{
			NALUs: [][]byte{
				{1}, // non-IDR
			},
			PTS: 4 * time.Second,
			DTS: 4 * time.Second,
		},

		{
			NALUs: [][]byte{
				{1}, // non-IDR
			},
			PTS: 6 * time.Second,
			DTS: 6 * time.Second,
		},
		{
			NALUs: [][]byte{
				{5}, // IDR
			},
			PTS: 7 * time.Second,
			DTS: 7 * time.Second,
		},
	}

	testAudioSamples := []*AudioSample{
		{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 3 * time.Second,
		},
		{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 3500 * time.Millisecond,
		},
		{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 4500 * time.Millisecond,
		},
	}

	for i, sample := range testVideoSamples {
		sample.IDRPresent = h264.IDRPresent(sample.NALUs)
		if i != len(testVideoSamples)-1 {
			sample.Next = testVideoSamples[i+1]
		}
	}
	testVideoSamples = testVideoSamples[:len(testVideoSamples)-1]

	for i, sample := range testAudioSamples {
		if i != len(testAudioSamples)-1 {
			sample.Next = testAudioSamples[i+1]
		}
	}
	testAudioSamples = testAudioSamples[:len(testAudioSamples)-1]

	t.Run("video + audio", func(t *testing.T) {
		byts, err := PartWrite(testVideoTrack, testAudioTrack, testVideoSamples, testAudioSamples)
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeMoof()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
			{gomp4.BoxTypeMdat()},
		}
		testMP4(t, byts, boxes)
	})

	t.Run("video only", func(t *testing.T) {
		byts, err := PartWrite(testVideoTrack, nil, testVideoSamples, nil)
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeMoof()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
			{gomp4.BoxTypeMdat()},
		}
		testMP4(t, byts, boxes)
	})

	t.Run("audio only", func(t *testing.T) {
		byts, err := PartWrite(nil, testAudioTrack, nil, testAudioSamples)
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeMoof()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
			{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
			{gomp4.BoxTypeMdat()},
		}
		testMP4(t, byts, boxes)
	})
}
