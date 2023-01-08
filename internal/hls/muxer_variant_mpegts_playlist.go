package hls

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type muxerVariantMPEGTSPlaylist struct {
	segmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []*muxerVariantMPEGTSSegment
	segmentByName      map[string]*muxerVariantMPEGTSSegment
	segmentDeleteCount int
}

func newMuxerVariantMPEGTSPlaylist(segmentCount int) *muxerVariantMPEGTSPlaylist {
	p := &muxerVariantMPEGTSPlaylist{
		segmentCount:  segmentCount,
		segmentByName: make(map[string]*muxerVariantMPEGTSSegment),
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *muxerVariantMPEGTSPlaylist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()
}

func (p *muxerVariantMPEGTSPlaylist) file(name string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader()

	case strings.HasSuffix(name, ".ts"):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerVariantMPEGTSPlaylist) playlist() io.Reader {
	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-VERSION:3\n"
	cnt += "#EXT-X-ALLOW-CACHE:NO\n"

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

	for _, s := range p.segments {
		cnt += "#EXT-X-PROGRAM-DATE-TIME:" + s.startTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n" +
			"#EXTINF:" + strconv.FormatFloat(s.duration().Seconds(), 'f', -1, 64) + ",\n" +
			s.name + ".ts\n"
	}

	return bytes.NewReader([]byte(cnt))
}

func (p *muxerVariantMPEGTSPlaylist) playlistReader() *MuxerFileResponse {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.closed && len(p.segments) == 0 {
		p.cond.Wait()
	}

	if p.closed {
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: p.playlist(),
	}
}

func (p *muxerVariantMPEGTSPlaylist) segmentReader(fname string) *MuxerFileResponse {
	base := strings.TrimSuffix(fname, ".ts")

	p.mutex.Lock()
	f, ok := p.segmentByName[base]
	p.mutex.Unlock()

	if !ok {
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": "video/MP2T",
		},
		Body: f.reader(),
	}
}

func (p *muxerVariantMPEGTSPlaylist) pushSegment(t *muxerVariantMPEGTSSegment) {
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
