package stream

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

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
	} else {
		if ss.CurDesc == nil {
			return fmt.Errorf("CurDesc must be non-nil")
		}
		if len(ss.CurDesc.Medias) != len(ss.Stream.Desc.Medias) {
			return fmt.Errorf("CurDesc must have the same number of medias as OrigDesc")
		}
	}

	if !ss.Stream.AlwaysAvailable {
		ss.CurDesc = ss.Stream.Desc
	}

	ss.medias = make(map[*description.Media]*subStreamMedia)

	ss.Stream.mutex.Lock()
	defer ss.Stream.mutex.Unlock()

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
			ss.Stream.offlineSubStream.close()
			ss.Stream.offlineSubStream = nil
		}
	}

	ss.Stream.subStream = ss

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
