package tracks

import (
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

// Write writes track informations.
func Write(w *message.ReadWriter, videoTrack formats.Format, audioTrack formats.Format) error {
	err := w.Write(&message.DataAMF0{
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
						case *formats.MPEG2Audio:
							return message.CodecMPEG2Audio

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

			err = w.Write(&message.Video{
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

		err = w.Write(&message.Audio{
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
