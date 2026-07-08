package stream

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func formatMPEG4AudioConfig(asc *mpeg4audio.AudioSpecificConfig) string {
	return fmt.Sprintf("type=%d, sampleRate=%d, channelCount=%d",
		asc.Type, asc.SampleRate, asc.ChannelConfig)
}

func formatG711Config(f *format.G711) string {
	return fmt.Sprintf("MULaw=%v, sampleRate=%d, channelCount=%d",
		f.MULaw, f.SampleRate, f.ChannelCount)
}

func formatLPCMConfig(f *format.LPCM) string {
	return fmt.Sprintf("bitDepth=%d, sampleRate=%d, channelCount=%d",
		f.BitDepth, f.SampleRate, f.ChannelCount)
}

func mediasAreCompatible(medias1 []*description.Media, medias2 []*description.Media) error {
	if len(medias1) != len(medias2) {
		return fmt.Errorf("wants to publish %v, but stream expects %v",
			formatlabel.MediasToLabels(medias2), formatlabel.MediasToLabels(medias1))
	}

	for i := range medias1 {
		if len(medias1[i].Formats) != len(medias2[i].Formats) {
			return fmt.Errorf("wants to publish %v, but stream expects %v",
				formatlabel.MediasToLabels(medias2), formatlabel.MediasToLabels(medias1))
		}

		for j := range medias1[i].Formats {
			if reflect.TypeOf(medias1[i].Formats[j]) != reflect.TypeOf(medias2[i].Formats[j]) {
				return fmt.Errorf("wants to publish %v, but stream expects %v",
					formatlabel.MediasToLabels(medias2), formatlabel.MediasToLabels(medias1))
			}
		}
	}

	for i := range medias1 {
		for j := range medias1[i].Formats {
			switch format1 := medias1[i].Formats[j].(type) {
			case *format.MPEG4Audio:
				format2 := medias2[i].Formats[j].(*format.MPEG4Audio)

				if !reflect.DeepEqual(format1.Config, format2.Config) {
					return fmt.Errorf("MPEG-4 audio configuration does not match, is %s, but stream expects %s",
						formatMPEG4AudioConfig(format2.Config), formatMPEG4AudioConfig(format1.Config))
				}

			case *format.G711:
				format2 := medias2[i].Formats[j].(*format.G711)

				if format1.MULaw != format2.MULaw ||
					format1.SampleRate != format2.SampleRate ||
					format1.ChannelCount != format2.ChannelCount {
					return fmt.Errorf("G711 configuration does not match, is %s, but stream expects %s",
						formatG711Config(format2), formatG711Config(format1))
				}

			case *format.LPCM:
				format2 := medias2[i].Formats[j].(*format.LPCM)

				if format1.BitDepth != format2.BitDepth ||
					format1.SampleRate != format2.SampleRate ||
					format1.ChannelCount != format2.ChannelCount {
					return fmt.Errorf("LPCM configuration does not match, is %s, but stream expects %s",
						formatLPCMConfig(format2), formatLPCMConfig(format1))
				}
			}
		}
	}

	return nil
}

func isKeyframeUnit(u *unit.Unit) bool {
	switch p := u.Payload.(type) {
	case unit.PayloadH264:
		for _, nalu := range p {
			if len(nalu) != 0 && nalu[0]&0x1F == 5 { // NAL type IDR
				return true
			}
		}
		return false

	case unit.PayloadH265:
		for _, nalu := range p {
			if len(nalu) != 0 {
				naluType := (nalu[0] >> 1) & 0x3F
				if naluType == 19 || naluType == 20 { // IDR_W_RADL, IDR_N_LP
					return true
				}
			}
		}
		return false

	default:
		return true
	}
}

// SubStream is a Stream without interruptions.
type SubStream struct {
	Stream        *Stream
	InDesc        *description.Session
	UseRTPPackets bool
	FallbackSwap  bool
	LiveSource    bool // if true, PTS is passed through unchanged (no ptsOffset applied on activation)

	medias            map[*description.Media]*subStreamMedia
	pendingActivation atomic.Bool
}

// SetupFormats initializes per-format codec state without making this SubStream active.
func (ss *SubStream) SetupFormats() error {
	swapMode := ss.Stream.AlwaysAvailable || ss.FallbackSwap

	if !swapMode {
		if ss.Stream.subStream != nil {
			panic("should not happen")
		}
		if ss.InDesc != nil {
			panic("should not happen")
		}
	} else {
		if ss.InDesc == nil {
			panic("should not happen")
		}
		err := mediasAreCompatible(ss.Stream.OrigDesc.Medias, ss.InDesc.Medias)
		if err != nil {
			return err
		}
	}

	if !swapMode {
		ss.InDesc = ss.Stream.OrigDesc
	}

	ss.medias = make(map[*description.Media]*subStreamMedia)

	for i, inMedia := range ss.InDesc.Medias {
		origMedia := ss.Stream.OrigDesc.Medias[i]

		ssm := &subStreamMedia{
			inMedia:       inMedia,
			streamMedia:   ss.Stream.medias[origMedia],
			useRTPPackets: ss.UseRTPPackets,
		}
		err := ssm.initialize()
		if err != nil {
			return err
		}
		ss.medias[inMedia] = ssm
	}

	if ss.Stream.AlwaysAvailable {
		if ss.Stream.offlineSubStream != nil {
			ss.Stream.Parent.Log(logger.Info, "stream is online")

			// wait for the entire duration of the last sample of the offline sub stream
			// to minimize errors in clients.
			// TODO: it would be better in the future to wait for the last sample
			// of normal sub streams as well (this is currently impossible because
			// we don't know the duration of their samples).
			ss.Stream.offlineSubStream.close(true)
			ss.Stream.offlineSubStream = nil
		}
	}

	return nil
}

func (ss *SubStream) activate() {
	ss.Stream.mutex.Lock()
	ss.Stream.subStream = ss
	ss.Stream.mutex.Unlock()

	for _, ssm := range ss.medias {
		for _, ssf := range ssm.formats {
			ssf.initialize2(ss.LiveSource, ss.Stream.firstTimeReceived, ss.Stream.lastPTS, ss.Stream.lastSystemTime)
		}
	}
}

// ScheduleActivation makes this SubStream become active on the next keyframe.
func (ss *SubStream) ScheduleActivation() {
	ss.pendingActivation.Store(true)
}

// Activate makes this SubStream the active source immediately.
func (ss *SubStream) Activate() {
	ss.activate()
}

// Initialize calls SetupFormats then activate immediately.
func (ss *SubStream) Initialize() error {
	if err := ss.SetupFormats(); err != nil {
		return err
	}
	ss.activate()
	return nil
}

func (ss *SubStream) hasVideo() bool {
	for _, m := range ss.InDesc.Medias {
		if m.Type == description.MediaTypeVideo {
			return true
		}
	}
	return false
}

// WriteUnit writes a Unit.
func (ss *SubStream) WriteUnit(inMedia *description.Media, inFormat format.Format, u *unit.Unit) {
	if ss.pendingActivation.Load() {
		if ss.hasVideo() {
			// require a video IDR; drop all audio and non-IDR video until then
			if inMedia.Type != description.MediaTypeVideo || !isKeyframeUnit(u) {
				return
			}
		} else if !isKeyframeUnit(u) {
			return
		}
		if ss.pendingActivation.CompareAndSwap(true, false) {
			ss.activate()
		}
	}

	ss.Stream.mutex.RLock()
	defer ss.Stream.mutex.RUnlock()

	if ss.Stream.subStream != ss {
		return
	}

	ssm := ss.medias[inMedia]
	ssf := ssm.formats[inFormat]

	ssf.writeUnit(u)
}
