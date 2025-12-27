package webrtc

import (
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpav1"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtplpcm"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpvp8"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpvp9"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const (
	webrtcPayloadMaxSize = 1188 // 1200 - 12 (RTP header)
)

var multichannelOpusSDP = map[int]string{
	3: "channel_mapping=0,2,1;num_streams=2;coupled_streams=1",
	4: "channel_mapping=0,1,2,3;num_streams=2;coupled_streams=2",
	5: "channel_mapping=0,4,1,2,3;num_streams=3;coupled_streams=2",
	6: "channel_mapping=0,4,1,2,3,5;num_streams=4;coupled_streams=2",
	7: "channel_mapping=0,4,1,2,3,5,6;num_streams=4;coupled_streams=4",
	8: "channel_mapping=0,6,1,4,5,2,3,7;num_streams=5;coupled_streams=4",
}

var errNoSupportedCodecsFrom = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"AV1, VP9, VP8, H265, H264, Opus, G722, G711, LPCM")

func ptrOf[T any](v T) *T {
	return &v
}

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(t), time.Second, time.Duration(clockRate))
}

func setupVideoTrack(
	desc *description.Session,
	r *stream.Reader,
	pc *PeerConnection,
) (format.Format, error) {
	var av1Format *format.AV1
	media := desc.FindFormat(&av1Format)

	if av1Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpav1.Encoder{
			PayloadType:    105,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		r.OnData(
			media,
			av1Format,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadAV1))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp), 90000))
					pkt.Timestamp += u.RTPPackets[0].Timestamp
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return av1Format, nil
	}

	var vp9Format *format.VP9
	media = desc.FindFormat(&vp9Format)

	if vp9Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpvp9.Encoder{
			PayloadType:      96,
			PayloadMaxSize:   webrtcPayloadMaxSize,
			InitialPictureID: ptrOf(uint16(8445)),
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		r.OnData(
			media,
			vp9Format,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadVP9))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp), 90000))
					pkt.Timestamp += u.RTPPackets[0].Timestamp
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return vp9Format, nil
	}

	var vp8Format *format.VP8
	media = desc.FindFormat(&vp8Format)

	if vp8Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpvp8.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		r.OnData(
			media,
			vp8Format,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadVP8))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp), 90000))
					pkt.Timestamp += u.RTPPackets[0].Timestamp
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return vp8Format, nil
	}

	var h265Format *format.H265
	media = desc.FindFormat(&h265Format)

	if h265Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH265,
				ClockRate:   90000,
				SDPFmtpLine: "level-id=93;profile-id=1;tier-flag=0;tx-mode=SRST",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtph265.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		firstReceived := false
		var lastPTS int64

		r.OnData(
			media,
			h265Format,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if u.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H265 streams with B-frames")
				}
				lastPTS = u.PTS

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadH265))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp), 90000))
					pkt.Timestamp += u.RTPPackets[0].Timestamp
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return h265Format, nil
	}

	var h264Format *format.H264
	media = desc.FindFormat(&h264Format)

	if h264Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtph264.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		firstReceived := false
		var lastPTS int64

		r.OnData(
			media,
			h264Format,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if u.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H264 streams with B-frames")
				}
				lastPTS = u.PTS

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadH264))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp), 90000))
					pkt.Timestamp += u.RTPPackets[0].Timestamp
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return h264Format, nil
	}

	return nil, nil
}

func setupAudioTrack(
	desc *description.Session,
	r *stream.Reader,
	pc *PeerConnection,
) (format.Format, error) {
	var opusFormat *format.Opus
	media := desc.FindFormat(&opusFormat)

	if opusFormat != nil {
		var caps webrtc.RTPCodecCapability

		switch opusFormat.ChannelCount {
		case 1, 2:
			caps = webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
				SDPFmtpLine: func() string {
					s := "minptime=10;useinbandfec=1"
					if opusFormat.ChannelCount == 2 {
						s += ";stereo=1;sprop-stereo=1"
					}
					return s
				}(),
			}

		case 3, 4, 5, 6, 7, 8:
			caps = webrtc.RTPCodecCapability{
				MimeType:    mimeTypeMultiopus,
				ClockRate:   48000,
				Channels:    uint16(opusFormat.ChannelCount),
				SDPFmtpLine: multichannelOpusSDP[opusFormat.ChannelCount],
			}

		default:
			return nil, fmt.Errorf("unsupported channel count: %d", opusFormat.ChannelCount)
		}

		track := &OutgoingTrack{
			Caps: caps,
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		curTimestamp, err := randUint32()
		if err != nil {
			return nil, err
		}

		r.OnData(
			media,
			opusFormat,
			func(u *unit.Unit) error {
				for _, orig := range u.RTPPackets {
					pkt := &rtp.Packet{
						Header:  orig.Header,
						Payload: orig.Payload,
					}

					// recompute timestamp from scratch.
					// Chrome requires a precise timestamp that FFmpeg doesn't provide.
					pkt.Timestamp = curTimestamp
					curTimestamp += uint32(opus.PacketDuration2(pkt.Payload))

					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp-u.RTPPackets[0].Timestamp), 48000))
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return opusFormat, nil
	}

	var g722Format *format.G722
	media = desc.FindFormat(&g722Format)

	if g722Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: 8000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		r.OnData(
			media,
			g722Format,
			func(u *unit.Unit) error {
				for _, pkt := range u.RTPPackets {
					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp-u.RTPPackets[0].Timestamp), 8000))
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return g722Format, nil
	}

	var g711Format *format.G711
	media = desc.FindFormat(&g711Format)

	if g711Format != nil {
		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if g711Format.ClockRate() != 8000 && g711Format.ClockRate() != 16000 &&
			g711Format.ClockRate() != 32000 && g711Format.ClockRate() != 48000 {
			return nil, fmt.Errorf("unsupported clock rate: %d", g711Format.ClockRate())
		}
		if g711Format.ChannelCount != 1 && g711Format.ChannelCount != 2 {
			return nil, fmt.Errorf("unsupported channel count: %d", g711Format.ChannelCount)
		}

		var caps webrtc.RTPCodecCapability

		if g711Format.ClockRate() == 8000 {
			if g711Format.MULaw {
				if g711Format.ChannelCount != 1 {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMU,
						ClockRate: uint32(g711Format.ClockRate()),
						Channels:  uint16(g711Format.ChannelCount),
					}
				} else {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMU,
						ClockRate: 8000,
					}
				}
			} else {
				if g711Format.ChannelCount != 1 {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMA,
						ClockRate: uint32(g711Format.ClockRate()),
						Channels:  uint16(g711Format.ChannelCount),
					}
				} else {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMA,
						ClockRate: 8000,
					}
				}
			}
		} else {
			caps = webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(g711Format.ClockRate()),
				Channels:  uint16(g711Format.ChannelCount),
			}
		}

		track := &OutgoingTrack{
			Caps: caps,
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		if g711Format.ClockRate() == 8000 {
			curTimestamp, err := randUint32()
			if err != nil {
				return nil, err
			}

			r.OnData(
				media,
				g711Format,
				func(u *unit.Unit) error {
					for _, orig := range u.RTPPackets {
						pkt := &rtp.Packet{
							Header:  orig.Header,
							Payload: orig.Payload,
						}

						// recompute timestamp from scratch.
						// Chrome requires a precise timestamp that FFmpeg doesn't provide.
						pkt.Timestamp = curTimestamp
						curTimestamp += uint32(len(pkt.Payload)) / uint32(g711Format.ChannelCount)

						ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp-u.RTPPackets[0].Timestamp), 8000))
						track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
					}

					return nil
				})
		} else {
			encoder := &rtplpcm.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
				BitDepth:       16,
				ChannelCount:   g711Format.ChannelCount,
			}
			err := encoder.Init()
			if err != nil {
				return nil, err
			}

			curTimestamp, err := randUint32()
			if err != nil {
				return nil, err
			}

			r.OnData(
				media,
				g711Format,
				func(u *unit.Unit) error {
					if u.NilPayload() {
						return nil
					}

					var lpcm []byte
					if g711Format.MULaw {
						var mu g711.Mulaw
						mu.Unmarshal(u.Payload.(unit.PayloadG711))
						lpcm = mu
					} else {
						var al g711.Alaw
						al.Unmarshal(u.Payload.(unit.PayloadG711))
						lpcm = al
					}

					packets, err2 := encoder.Encode(lpcm)
					if err2 != nil {
						return nil //nolint:nilerr
					}

					for _, pkt := range packets {
						// recompute timestamp from scratch.
						// Chrome requires a precise timestamp that FFmpeg doesn't provide.
						pkt.Timestamp = curTimestamp
						curTimestamp += uint32(len(pkt.Payload)) / 2 / uint32(g711Format.ChannelCount)

						ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp-u.RTPPackets[0].Timestamp),
							g711Format.ClockRate()))
						track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
					}

					return nil
				})
		}

		return g711Format, nil
	}

	var lpcmFormat *format.LPCM
	media = desc.FindFormat(&lpcmFormat)

	if lpcmFormat != nil {
		if lpcmFormat.BitDepth != 16 {
			return nil, fmt.Errorf("unsupported LPCM bit depth: %d", lpcmFormat.BitDepth)
		}

		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if lpcmFormat.ClockRate() != 8000 && lpcmFormat.ClockRate() != 16000 &&
			lpcmFormat.ClockRate() != 32000 && lpcmFormat.ClockRate() != 48000 {
			return nil, fmt.Errorf("unsupported clock rate: %d", lpcmFormat.ClockRate())
		}
		if lpcmFormat.ChannelCount != 1 && lpcmFormat.ChannelCount != 2 {
			return nil, fmt.Errorf("unsupported channel count: %d", lpcmFormat.ChannelCount)
		}

		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(lpcmFormat.ClockRate()),
				Channels:  uint16(lpcmFormat.ChannelCount),
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtplpcm.Encoder{
			PayloadType:    96,
			BitDepth:       16,
			ChannelCount:   lpcmFormat.ChannelCount,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		curTimestamp, err := randUint32()
		if err != nil {
			return nil, err
		}

		r.OnData(
			media,
			lpcmFormat,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				packets, err2 := encoder.Encode(u.Payload.(unit.PayloadLPCM))
				if err2 != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					// recompute timestamp from scratch.
					// Chrome requires a precise timestamp that FFmpeg doesn't provide.
					pkt.Timestamp = curTimestamp
					curTimestamp += uint32(len(pkt.Payload)) / 2 / uint32(lpcmFormat.ChannelCount)

					ntp := u.NTP.Add(timestampToDuration(int64(pkt.Timestamp-u.RTPPackets[0].Timestamp),
						lpcmFormat.ClockRate()))
					track.WriteRTPWithNTP(pkt, ntp) //nolint:errcheck
				}

				return nil
			})

		return lpcmFormat, nil
	}

	return nil, nil
}

// FromStream maps a MediaMTX stream to a WebRTC connection
func FromStream(
	desc *description.Session,
	r *stream.Reader,
	pc *PeerConnection,
) error {
	videoFormat, err := setupVideoTrack(desc, r, pc)
	if err != nil {
		return err
	}

	audioFormat, err := setupAudioTrack(desc, r, pc)
	if err != nil {
		return err
	}

	if videoFormat == nil && audioFormat == nil {
		return errNoSupportedCodecsFrom
	}

	setuppedFormats := r.Formats()

	n := 1
	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			if !slices.Contains(setuppedFormats, forma) {
				r.Parent.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
