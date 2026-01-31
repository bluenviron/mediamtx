package stream

import (
	"fmt"
	"reflect"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// FormatsToCodecs returns the name of codecs of given formats.
func FormatsToCodecs(formats []format.Format) []string {
	ret := make([]string, len(formats))
	for i, forma := range formats {
		ret[i] = forma.Codec()
	}
	return ret
}

func gatherFormats(medias []*description.Media) []format.Format {
	n := 0
	for _, media := range medias {
		n += len(media.Formats)
	}

	if n == 0 {
		return nil
	}

	formats := make([]format.Format, n)
	n = 0

	for _, media := range medias {
		n += copy(formats[n:], media.Formats)
	}

	return formats
}

func mediasToCodecs(medias []*description.Media) []string {
	return FormatsToCodecs(gatherFormats(medias))
}

func mediasAreCompatible(medias1 []*description.Media, medias2 []*description.Media) bool {
	if len(medias1) != len(medias2) {
		return false
	}

	for i := range medias1 {
		if len(medias1[i].Formats) != len(medias2[i].Formats) {
			return false
		}

		for j := range medias1[i].Formats {
			if reflect.TypeOf(medias1[i].Formats[j]) != reflect.TypeOf(medias2[i].Formats[j]) {
				return false
			}
		}
	}

	return true
}

// SubStream is a Stream without interruptions.
type SubStream struct {
	Stream        *Stream
	CurDesc       *description.Session
	UseRTPPackets bool

	medias map[*description.Media]*subStreamMedia
}

// Initialize initializes the SubStream.
func (ss *SubStream) Initialize() error {
	if !ss.Stream.AlwaysAvailable {
		if ss.Stream.subStream != nil {
			panic("should not happen")
		}

		if ss.CurDesc != nil {
			panic("should not happen")
		}
	} else {
		if ss.CurDesc == nil {
			panic("should not happen")
		}

		if !mediasAreCompatible(ss.Stream.Desc.Medias, ss.CurDesc.Medias) {
			return fmt.Errorf("want to publish %v, but stream expects %v",
				mediasToCodecs(ss.CurDesc.Medias), mediasToCodecs(ss.Stream.Desc.Medias))
		}
	}

	if !ss.Stream.AlwaysAvailable {
		ss.CurDesc = ss.Stream.Desc
	}

	ss.medias = make(map[*description.Media]*subStreamMedia)

	for i, curMedia := range ss.CurDesc.Medias {
		media := ss.Stream.Desc.Medias[i]

		ssm := &subStreamMedia{
			curMedia:      curMedia,
			streamMedia:   ss.Stream.medias[media],
			useRTPPackets: ss.UseRTPPackets,
		}
		err := ssm.initialize()
		if err != nil {
			return err
		}
		ss.medias[curMedia] = ssm
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

	ss.Stream.mutex.Lock()
	ss.Stream.subStream = ss
	ss.Stream.mutex.Unlock()

	for _, ssm := range ss.medias {
		for _, ssf := range ssm.formats {
			ssf.initialize2()
		}
	}

	return nil
}

// WriteUnit writes a Unit.
func (ss *SubStream) WriteUnit(medi *description.Media, forma format.Format, u *unit.Unit) {
	ss.Stream.mutex.RLock()
	defer ss.Stream.mutex.RUnlock()

	if ss.Stream.subStream != ss {
		return
	}

	ssm := ss.medias[medi]
	ssf := ssm.formats[forma]

	ssf.writeUnit(u)
}
