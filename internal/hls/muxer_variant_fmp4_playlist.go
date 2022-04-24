package hls

import (
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/aler9/gortsplib"
)

type muxerVariantFMP4Playlist struct {
	segmentCount int
	videoTrack   *gortsplib.TrackH264
	audioTrack   *gortsplib.TrackAAC

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []*muxerVariantFMP4Segment
	segmentByName      map[string]*muxerVariantFMP4Segment
	segmentDeleteCount int
}

func newMuxerVariantFMP4Playlist(
	segmentCount int,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerVariantFMP4Playlist {
	p := &muxerVariantFMP4Playlist{
		segmentCount:  segmentCount,
		videoTrack:    videoTrack,
		audioTrack:    audioTrack,
		segmentByName: make(map[string]*muxerVariantFMP4Segment),
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *muxerVariantFMP4Playlist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()
}

func (p *muxerVariantFMP4Playlist) playlistReader() io.Reader {
	return &asyncReader{generator: func() []byte {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if !p.closed && len(p.segments) == 0 {
			p.cond.Wait()
		}

		if p.closed {
			return nil
		}

		/*
			#EXTM3U
			#EXT-X-VERSION:7
			#EXT-X-TARGETDURATION:8
			#EXT-X-MEDIA-SEQUENCE:0
			#EXT-X-MAP:URI="init.mp4"
			#EXTINF:8.333333,
			index0.m4s
			#EXTINF:8.333333,
			index1.m4s
		*/

		cnt := "#EXTM3U\n"
		cnt += "#EXT-X-VERSION:7\n"

		targetDuration := func() uint {
			ret := uint(0)

			// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
			for _, s := range p.segments {
				v2 := uint(math.Round(s.duration().Seconds()))
				if v2 > ret {
					ret = v2
				}
			}

			return ret
		}()
		cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

		cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(p.segmentDeleteCount), 10) + "\n"
		cnt += "#EXT-X-INDEPENDENT-SEGMENTS" + "\n"
		cnt += "#EXT-X-MAP:URI=\"init.mp4\"\n"
		cnt += "\n"

		for _, s := range p.segments {
			cnt += "#EXT-X-PROGRAM-DATE-TIME:" + s.startTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n" +
				"#EXTINF:" + strconv.FormatFloat(s.duration().Seconds(), 'f', -1, 64) + ",\n" +
				s.name + ".m4s\n"
		}

		return []byte(cnt)
	}}
}

func (p *muxerVariantFMP4Playlist) segmentReader(fname string) io.Reader {
	if fname == "init.mp4" {
		return &asyncReader{generator: func() []byte {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			byts, err := mp4InitGenerate(p.videoTrack, p.audioTrack)
			if err != nil {
				return nil
			}

			return byts
		}}
	}

	base := strings.TrimSuffix(fname, ".m4s")

	p.mutex.Lock()
	f, ok := p.segmentByName[base]
	p.mutex.Unlock()

	if !ok {
		return nil
	}

	return f.reader()
}

func (p *muxerVariantFMP4Playlist) pushSegment(t *muxerVariantFMP4Segment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.segmentByName[t.name] = t
		p.segments = append(p.segments, t)

		if len(p.segments) > p.segmentCount {
			delete(p.segmentByName, p.segments[0].name)
			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}
