package rtmp

import (
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

func mpeg1AudioRate(sr int) uint8 {
	switch sr {
	case 5500:
		return flvio.SOUND_5_5Khz
	case 11025:
		return flvio.SOUND_11Khz
	case 22050:
		return flvio.SOUND_22Khz
	default:
		return flvio.SOUND_44Khz
	}
}

func mpeg1AudioChannels(m mpeg1audio.ChannelMode) uint8 {
	if m == mpeg1audio.ChannelModeMono {
		return flvio.SOUND_MONO
	}
	return flvio.SOUND_STEREO
}

// Writer is a wrapper around Conn that provides utilities to mux outgoing data.
type Writer struct {
	conn *Conn
}

// NewWriter allocates a Writer.
func NewWriter(conn *Conn, videoTrack formats.Format, audioTrack formats.Format) (*Writer, error) {
	w := &Writer{
		conn: conn,
	}

	err := w.writeTracks(videoTrack, audioTrack)
	if err != nil {
		return nil, err
	}

	return w, nil
}

func (w *Writer) writeTracks(videoTrack formats.Format, audioTrack formats.Format) error {
	err := w.conn.Write(&message.DataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Payload: []interface{}{
			"@setDataFrame",
			"onMetaData",
			flvio.AMFMap{
				{
					K: "videodatarate",
					V: float64(0),
				},
				{
					K: "videocodecid",
					V: func() float64 {
						switch videoTrack.(type) {
						case *formats.H264:
							return message.CodecH264

						default:
							return 0
						}
					}(),
				},
				{
					K: "audiodatarate",
					V: float64(0),
				},
				{
					K: "audiocodecid",
					V: func() float64 {
						switch audioTrack.(type) {
						case *formats.MPEG1Audio:
							return message.CodecMPEG1Audio

						case *formats.MPEG4AudioGeneric, *formats.MPEG4AudioLATM:
							return message.CodecMPEG4Audio

						default:
							return 0
						}
					}(),
				},
			},
		},
	})
	if err != nil {
		return err
	}

	if videoTrack, ok := videoTrack.(*formats.H264); ok {
		// write decoder config only if SPS and PPS are available.
		// if they're not available yet, they're sent later.
		if sps, pps := videoTrack.SafeParams(); sps != nil && pps != nil {
			buf, _ := h264conf.Conf{
				SPS: sps,
				PPS: pps,
			}.Marshal()

			err = w.conn.Write(&message.Video{
				ChunkStreamID:   message.VideoChunkStreamID,
				MessageStreamID: 0x1000000,
				Codec:           message.CodecH264,
				IsKeyFrame:      true,
				Type:            message.VideoTypeConfig,
				Payload:         buf,
			})
			if err != nil {
				return err
			}
		}
	}

	var audioConfig *mpeg4audio.AudioSpecificConfig

	switch track := audioTrack.(type) {
	case *formats.MPEG4Audio:
		audioConfig = track.Config

	case *formats.MPEG4AudioLATM:
		audioConfig = track.Config.Programs[0].Layers[0].AudioSpecificConfig
	}

	if audioConfig != nil {
		enc, err := audioConfig.Marshal()
		if err != nil {
			return err
		}

		err = w.conn.Write(&message.Audio{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: 0x1000000,
			Codec:           message.CodecMPEG4Audio,
			Rate:            flvio.SOUND_44Khz,
			Depth:           flvio.SOUND_16BIT,
			Channels:        flvio.SOUND_STEREO,
			AACType:         message.AudioAACTypeConfig,
			Payload:         enc,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteH264 writes H264 data.
func (w *Writer) WriteH264(pts time.Duration, dts time.Duration, idrPresent bool, au [][]byte) error {
	avcc, err := h264.AVCCMarshal(au)
	if err != nil {
		return err
	}

	return w.conn.Write(&message.Video{
		ChunkStreamID:   message.VideoChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecH264,
		IsKeyFrame:      idrPresent,
		Type:            message.VideoTypeAU,
		Payload:         avcc,
		DTS:             dts,
		PTSDelta:        pts - dts,
	})
}

// WriteMPEG4Audio writes MPEG-4 Audio data.
func (w *Writer) WriteMPEG4Audio(pts time.Duration, au []byte) error {
	return w.conn.Write(&message.Audio{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecMPEG4Audio,
		Rate:            flvio.SOUND_44Khz,
		Depth:           flvio.SOUND_16BIT,
		Channels:        flvio.SOUND_STEREO,
		AACType:         message.AudioAACTypeAU,
		Payload:         au,
		DTS:             pts,
	})
}

// WriteMPEG1Audio writes MPEG-1 Audio data.
func (w *Writer) WriteMPEG1Audio(pts time.Duration, h *mpeg1audio.FrameHeader, frame []byte) error {
	return w.conn.Write(&message.Audio{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecMPEG1Audio,
		Rate:            mpeg1AudioRate(h.SampleRate),
		Depth:           flvio.SOUND_16BIT,
		Channels:        mpeg1AudioChannels(h.ChannelMode),
		Payload:         frame,
		DTS:             pts,
	})
}
