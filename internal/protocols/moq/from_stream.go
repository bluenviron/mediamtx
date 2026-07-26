// Package moq contains Media-over-QUIC utilities.
package moq

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"
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
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type writeDataFunc func(payload []byte, pts int64) error

// SetupTrackFunc is a function that sets up a track in a MediaMTX stream.
type SetupTrackFunc func(r *stream.Reader, writeData writeDataFunc)

// FromStream maps a MediaMTX stream to a Media-over-QUIC catalog and subscribed tracks.
func FromStream(desc *description.Session) (*catalog.Catalog, []SetupTrackFunc, error) {
	cat := &catalog.Catalog{
		Version: 1,
	}

	var setupTracks []SetupTrackFunc

	addTrack := func(
		media *description.Media,
		forma format.Format,
		track catalog.Track,
		genParsePayload func(writeData writeDataFunc) func(u *unit.Unit) error,
	) {
		track.Name = strconv.Itoa(len(cat.Tracks))
		track.Packaging = "loc"
		track.IsLive = true

		setup := func(r *stream.Reader, writeData writeDataFunc) {
			parsePayload := genParsePayload(writeData)

			r.OnData(media, forma, func(u *unit.Unit) error {
				return parsePayload(u)
			})
		}

		cat.Tracks = append(cat.Tracks, track)
		setupTracks = append(setupTracks, setup)
	}

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.AV1:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec: "av01.0.04M.08",
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						firstRandomAccess := false

						return func(u *unit.Unit) error {
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

							return writeData(payload, u.PTS)
						}
					},
				)

			case *format.VP9:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec: "vp09.00.10.08",
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						firstRandomAccess := false

						return func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							if !firstRandomAccess && !vp9.IsRandomAccess(u.Payload.(unit.PayloadVP9)) {
								return nil
							}
							firstRandomAccess = true

							return writeData(u.Payload.(unit.PayloadVP9), u.PTS)
						}
					},
				)

			case *format.VP8:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec: "vp8",
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						firstRandomAccess := false

						return func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							if !firstRandomAccess && !vp8.IsRandomAccess(u.Payload.(unit.PayloadVP8)) {
								return nil
							}
							firstRandomAccess = true

							return writeData(u.Payload.(unit.PayloadVP8), u.PTS)
						}
					},
				)

			case *format.H265:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec: "hev1.1.6.L93.B0",
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						firstRandomAccess := false

						return func(u *unit.Unit) error {
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

							return writeData(payload, u.PTS)
						}
					},
				)

			case *format.H264:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec: "avc3.640028",
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						firstRandomAccess := false

						return func(u *unit.Unit) error {
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

							return writeData(payload, u.PTS)
						}
					},
				)

			case *format.Opus:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec:      "opus",
						Samplerate: 48000,
						Channels:   forma.ChannelCount,
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						return func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							for _, pkt := range u.Payload.(unit.PayloadOpus) {
								err := writeData(pkt, u.PTS)
								if err != nil {
									return err
								}
							}
							return nil
						}
					},
				)

			case *format.Generic:
				if strings.HasPrefix(strings.ToLower(forma.RTPMap()), "flac/") {
					enc, err := hex.DecodeString(forma.FMT["streaminfo"])
					if err != nil {
						return nil, nil, err
					}

					var streamInfo flac.StreamInfo
					err = streamInfo.Unmarshal(enc)
					if err != nil {
						return nil, nil, err
					}

					addTrack(
						media,
						forma,
						catalog.Track{
							Codec:      "flac",
							Samplerate: int(streamInfo.SampleRate),
							Channels:   int(streamInfo.ChannelCount),
							InitData:   base64.StdEncoding.EncodeToString(enc),
						},
						func(writeData writeDataFunc) func(u *unit.Unit) error {
							return func(u *unit.Unit) error {
								if u.NilPayload() {
									return nil
								}

								return writeData(u.Payload.(unit.PayloadFLAC), u.PTS)
							}
						},
					)
				}

			case *format.MPEG4Audio:
				if forma.Config != nil {
					enc, err := forma.Config.Marshal()
					if err != nil {
						return nil, nil, err
					}

					addTrack(
						media,
						forma,
						catalog.Track{
							Codec:      "mp4a.40.2",
							Samplerate: forma.Config.SampleRate,
							Channels:   int(forma.Config.ChannelConfig),
							InitData:   base64.StdEncoding.EncodeToString(enc),
						},
						func(writeData writeDataFunc) func(u *unit.Unit) error {
							return func(u *unit.Unit) error {
								if u.NilPayload() {
									return nil
								}

								pts := u.PTS

								for _, au := range u.Payload.(unit.PayloadMPEG4Audio) {
									err2 := writeData(au, pts)
									if err2 != nil {
										return err2
									}

									pts += mpeg4audio.SamplesPerAccessUnit
								}
								return nil
							}
						},
					)
				}

			case *format.G711:
				addTrack(
					media,
					forma,
					catalog.Track{
						Codec:      "pcm-s16",
						Samplerate: forma.SampleRate,
						Channels:   forma.ChannelCount,
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						return func(u *unit.Unit) error {
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

							return writeData(swapped, u.PTS)
						}
					},
				)

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

				addTrack(
					media,
					forma,
					catalog.Track{
						Codec:      codec,
						Samplerate: forma.SampleRate,
						Channels:   forma.ChannelCount,
					},
					func(writeData writeDataFunc) func(u *unit.Unit) error {
						return func(u *unit.Unit) error {
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

							return writeData(swapped, u.PTS)
						}
					},
				)
			}
		}
	}

	return cat, setupTracks, nil
}
