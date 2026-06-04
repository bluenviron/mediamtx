// Package moq contains Media-over-QUIC utilities.
package moq

import (
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/flac"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/vp8"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/vp9"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// FromStream maps a MediaMTX stream to a Media-over-QUIC stream.
func FromStream(desc *description.Session) ([]*Track, error) {
	var tracks []*Track

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.AV1:
				firstRandomAccess := false

				track := &Track{
					Codec:  "av01.0.04M.08",
					Media:  media,
					Format: forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						if !firstRandomAccess && !av1.IsRandomAccess2(u.Payload.(unit.PayloadAV1)) {
							return nil
						}
						firstRandomAccess = true

						payload, err := av1.Bitstream([][]byte(u.Payload.(unit.PayloadAV1))).Marshal()
						if err != nil {
							return err
						}

						return wrapped(payload, u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.VP9:
				firstRandomAccess := false

				track := &Track{
					Codec:  "vp09.00.10.08",
					Media:  media,
					Format: forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						if !firstRandomAccess && !vp9.IsRandomAccess(u.Payload.(unit.PayloadVP9)) {
							return nil
						}
						firstRandomAccess = true

						return wrapped(u.Payload.(unit.PayloadVP9), u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.VP8:
				firstRandomAccess := false

				track := &Track{
					Codec:  "vp8",
					Media:  media,
					Format: forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						if !firstRandomAccess && !vp8.IsRandomAccess(u.Payload.(unit.PayloadVP8)) {
							return nil
						}
						firstRandomAccess = true

						return wrapped(u.Payload.(unit.PayloadVP8), u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.H265:
				firstRandomAccess := false

				track := &Track{
					Codec:  "hev1.1.6.L93.B0",
					Media:  media,
					Format: forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						if !firstRandomAccess && !h265.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
							return nil
						}
						firstRandomAccess = true

						payload, err := h264.AVCC(u.Payload.(unit.PayloadH265)).Marshal()
						if err != nil {
							return err
						}

						return wrapped(payload, u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.H264:
				firstRandomAccess := false

				track := &Track{
					Codec:  "avc3.640028",
					Media:  media,
					Format: forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						if !firstRandomAccess && !h264.IsRandomAccess(u.Payload.(unit.PayloadH264)) {
							return nil
						}
						firstRandomAccess = true

						payload, err := h264.AVCC(u.Payload.(unit.PayloadH264)).Marshal()
						if err != nil {
							return err
						}

						return wrapped(payload, u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.Opus:
				track := &Track{
					Codec:      "opus",
					Samplerate: 48000,
					Channels:   forma.ChannelCount,
					Media:      media,
					Format:     forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						for _, pkt := range u.Payload.(unit.PayloadOpus) {
							err := wrapped(pkt, u.PTS)
							if err != nil {
								return err
							}
						}
						return nil
					},
				}
				tracks = append(tracks, track)

			case *format.Generic:
				if strings.HasPrefix(strings.ToLower(forma.RTPMap()), "flac/") {
					enc, err := hex.DecodeString(forma.FMT["streaminfo"])
					if err != nil {
						return nil, err
					}

					var streamInfo flac.StreamInfo
					err = streamInfo.Unmarshal(enc)
					if err != nil {
						return nil, err
					}

					track := &Track{
						Codec:      "flac",
						Samplerate: int(streamInfo.SampleRate),
						Channels:   int(streamInfo.ChannelCount),
						InitData:   base64.StdEncoding.EncodeToString(enc),
						Media:      media,
						Format:     forma,
						OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
							if u.NilPayload() {
								return nil
							}

							return wrapped(u.Payload.(unit.PayloadFLAC), u.PTS)
						},
					}
					tracks = append(tracks, track)
				}

			case *format.MPEG4Audio:
				if forma.Config != nil {
					enc, err := forma.Config.Marshal()
					if err != nil {
						return nil, err
					}

					track := &Track{
						Codec:      "mp4a.40.2",
						Samplerate: forma.Config.SampleRate,
						Channels:   int(forma.Config.ChannelConfig),
						InitData:   base64.StdEncoding.EncodeToString(enc),
						Media:      media,
						Format:     forma,
						OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
							if u.NilPayload() {
								return nil
							}

							pts := u.PTS

							for _, au := range u.Payload.(unit.PayloadMPEG4Audio) {
								err2 := wrapped(au, pts)
								if err2 != nil {
									return err2
								}

								pts += mpeg4audio.SamplesPerAccessUnit
							}
							return nil
						},
					}
					tracks = append(tracks, track)
				}

			case *format.G711:
				track := &Track{
					Codec:      "pcm-s16",
					Samplerate: forma.SampleRate,
					Channels:   forma.ChannelCount,
					Media:      media,
					Format:     forma,
					OnData: func(u *unit.Unit, wrapped func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						var bigEndian []byte
						if forma.MULaw {
							var mu g711.Mulaw
							mu.Unmarshal(u.Payload.(unit.PayloadG711))
							bigEndian = mu
						} else {
							var al g711.Alaw
							al.Unmarshal(u.Payload.(unit.PayloadG711))
							bigEndian = al
						}

						swapped := make([]byte, len(bigEndian))
						for i := 0; i+2 <= len(bigEndian); i += 2 {
							swapped[i], swapped[i+1] = bigEndian[i+1], bigEndian[i]
						}

						return wrapped(swapped, u.PTS)
					},
				}
				tracks = append(tracks, track)

			case *format.LPCM:
				var codec string
				switch forma.BitDepth {
				case 8:
					codec = "pcm-u8"
				case 16:
					codec = "pcm-s16"
				case 24:
					codec = "pcm-s24"
				default: // 32
					codec = "pcm-s32"
				}

				track := &Track{
					Codec:      codec,
					Samplerate: forma.SampleRate,
					Channels:   forma.ChannelCount,
					Media:      media,
					Format:     forma,
					OnData: func(u *unit.Unit, onData func([]byte, int64) error) error {
						if u.NilPayload() {
							return nil
						}

						src := []byte(u.Payload.(unit.PayloadLPCM))
						byteDepth := forma.BitDepth / 8
						swapped := make([]byte, len(src))
						for i := 0; i+byteDepth <= len(src); i += byteDepth {
							for j := range byteDepth {
								swapped[i+j] = src[i+byteDepth-1-j]
							}
						}

						return onData(swapped, u.PTS)
					},
				}
				tracks = append(tracks, track)
			}
		}
	}

	return tracks, nil
}
