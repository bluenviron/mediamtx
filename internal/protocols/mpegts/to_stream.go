// Package mpegts contains MPEG-ts utilities.
package mpegts

import (
	"errors"
	"io"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"H265, H264, MPEG-4 Video, MPEG-1/2 Video, Opus, MPEG-4 Audio, MPEG-1 Audio, AC-3")

// mpegTSReader is a wrapper around mpegts.Reader that captures raw MPEG-TS data.
type mpegTSReader struct {
	r         io.Reader
	buffer    []byte
	stream    **stream.Stream
	media     *description.Media
	format    format.Format
	startTime time.Time
	sequence  uint16
}

func (r *mpegTSReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)

	// If we read any data, send it to the stream as raw MPEG-TS data
	if n > 0 {
		// Create a copy of the data to avoid issues with buffer reuse
		data := make([]byte, n)
		copy(data, p[:n])

		// Initialize startTime if this is the first packet
		if r.startTime.IsZero() {
			r.startTime = time.Now()
		}

		// Calculate PTS based on elapsed time since start (in 90kHz clock rate)
		now := time.Now()
		elapsed := now.Sub(r.startTime)
		pts := int64(elapsed.Milliseconds() * 90) // Convert to 90kHz clock rate

		// Create an RTP packet with the MPEG-TS data as payload
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: r.sequence,
				Timestamp:      uint32(pts),
				SSRC:           1234, // Fixed SSRC for consistency
			},
			Payload: data,
		}

		// Increment sequence number for next packet
		r.sequence++

		// Send the raw MPEG-TS data to the stream as a Generic unit
		(*r.stream).WriteUnit(r.media, r.format, &unit.Generic{
			Base: unit.Base{
				RTPPackets: []*rtp.Packet{pkt},
				NTP:        now,
				PTS:        pts,
			},
		})
	}

	return n, err
}

// ToStream maps a MPEG-TS stream to a MediaMTX stream.
func ToStream(
	r *mpegts.Reader,
	stream **stream.Stream,
	l logger.Writer,
) ([]*description.Media, error) {
	// Wrap the reader to capture raw MPEG-TS data for passthrough recording
	if _, ok := r.R.(*mpegTSReader); !ok {
		// Create a generic format for the raw MPEG-TS data
		genericFormat := &format.Generic{
			PayloadTyp: 96,
			RTPMa:      "private/90000",
		}
		// Initialize the format
		genericFormat.Init()

		// Create a media for the raw MPEG-TS data
		media := &description.Media{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{genericFormat},
		}

		r.R = &mpegTSReader{
			r:      r.R,
			stream: stream,
			media:  media,
			format: genericFormat,
		}
	}
	var medias []*description.Media //nolint:prealloc
	var unsupportedTracks []int

	td := &mpegts.TimeDecoder{}
	td.Initialize()

	for i, track := range r.Tracks() { //nolint:dupl
		var medi *description.Media

		switch codec := track.Codec.(type) {
		case *mpegts.CodecH265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH265(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecH264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH264(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Video:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG4Video{
					PayloadTyp: 96,
				}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Video:
			medi = &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG1Video{}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &codec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frames: frames,
				})
				return nil
			})

		case *mpegts.CodecAC3:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.AC3{
					PayloadTyp:   96,
					SampleRate:   codec.SampleRate,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataAC3(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.AC3{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					Frames: [][]byte{frame},
				})
				return nil
			})

		default:
			unsupportedTracks = append(unsupportedTracks, i+1)
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		// Even if there are no supported codecs, we can still record the raw MPEG-TS data
		// Create a dummy media to allow the stream to be created
		genericFormat := &format.Generic{
			PayloadTyp: 96,
			RTPMa:      "private/90000",
		}
		// Initialize the format to compute the clock rate
		err := genericFormat.Init()
		if err != nil {
			l.Log(logger.Warn, "failed to initialize generic format: %v", err)
		}

		media := &description.Media{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{genericFormat},
		}
		medias = append(medias, media)

		l.Log(logger.Info, "no supported codecs found, using MPEG-TS passthrough mode")
	}

	for _, id := range unsupportedTracks {
		l.Log(logger.Warn, "skipping track %d (unsupported codec)", id)
	}

	return medias, nil
}
