package hls

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/aler9/gortsplib"
)

func targetDuration(segments []*muxerVariantFMP4Segment) uint {
	ret := uint(0)

	// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
	for _, s := range segments {
		v2 := uint(math.Round(s.duration.Seconds()))
		if v2 > ret {
			ret = v2
		}
	}

	return ret
}

type muxerVariantFMP4Playlist struct {
	lowLatency   bool
	segmentCount int
	videoTrack   *gortsplib.TrackH264
	audioTrack   *gortsplib.TrackAAC

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []*muxerVariantFMP4Segment
	segmentsByName     map[string]*muxerVariantFMP4Segment
	segmentDeleteCount int
	parts              []*muxerVariantFMP4Part
	partsByName        map[string]*muxerVariantFMP4Part
	nextSegmentID      uint64
	nextSegmentParts   []*muxerVariantFMP4Part
	nextPartID         uint64
}

func newMuxerVariantFMP4Playlist(
	lowLatency bool,
	segmentCount int,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerVariantFMP4Playlist {
	p := &muxerVariantFMP4Playlist{
		lowLatency:     lowLatency,
		segmentCount:   segmentCount,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		segmentsByName: make(map[string]*muxerVariantFMP4Segment),
		partsByName:    make(map[string]*muxerVariantFMP4Part),
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

func (p *muxerVariantFMP4Playlist) hasContent() bool {
	// wait for at least 2 segments, otherwise most clients have problems
	return len(p.segments) >= 2
}

func (p *muxerVariantFMP4Playlist) hasPart(segmentID uint64, partID uint64) bool {
	if !p.hasContent() {
		return false
	}

	for _, seg := range p.segments {
		if segmentID != seg.id {
			continue
		}

		// If the Client requests a Part Index greater than that of the final
		// Partial Segment of the Parent Segment, the Server MUST treat the
		// request as one for Part Index 0 of the following Parent Segment.
		if partID >= uint64(len(seg.parts)) {
			segmentID++
			partID = 0
			continue
		}

		return true
	}

	if segmentID != p.nextSegmentID {
		return false
	}

	if partID >= uint64(len(p.nextSegmentParts)) {
		return false
	}

	return true
}

func (p *muxerVariantFMP4Playlist) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader(msn, part, skip)

	case strings.HasSuffix(name, ".mp4"):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerVariantFMP4Playlist) playlistReader(msn string, part string, skip string) *MuxerFileResponse {
	if p.lowLatency {
		var msnint uint64
		if msn != "" {
			var err error
			msnint, err = strconv.ParseUint(msn, 10, 64)
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}
		}

		var partint uint64
		if part != "" {
			var err error
			partint, err = strconv.ParseUint(part, 10, 64)
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}
		}

		if msn != "" {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			// If the _HLS_msn is greater than the Media Sequence Number of the last
			// Media Segment in the current Playlist plus two, or if the _HLS_part
			// exceeds the last Partial Segment in the current Playlist by the
			// Advance Part Limit, then the server SHOULD immediately return Bad
			// Request, such as HTTP 400.
			if msnint > (p.nextSegmentID + 1) {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}

			for !p.closed && !p.hasPart(msnint, partint) {
				p.cond.Wait()
			}

			if p.closed {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": `application/x-mpegURL`,
				},
				Body: p.fullPlaylist(),
			}
		}

		// part without msn is not supported.
		if part != "" {
			return &MuxerFileResponse{Status: http.StatusBadRequest}
		}
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	for !p.closed && !p.hasContent() {
		p.cond.Wait()
	}

	if p.closed {
		return &MuxerFileResponse{Status: http.StatusInternalServerError}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: p.fullPlaylist(),
	}
}

func (p *muxerVariantFMP4Playlist) fullPlaylist() io.Reader {
	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-VERSION:7\n"

	targetDuration := targetDuration(p.segments)
	cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

	if p.lowLatency {
		var part *muxerVariantFMP4Part
		if len(p.segments) > 0 {
			part = p.segments[0].parts[0]
		} else {
			part = p.nextSegmentParts[0]
		}

		// The value is an enumerated-string whose value is YES if the server
		// supports Blocking Playlist Reload
		cnt += "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES"

		// The value is a decimal-floating-point number of seconds that
		// indicates the server-recommended minimum distance from the end of
		// the Playlist at which clients should begin to play or to which
		// they should seek when playing in Low-Latency Mode.  Its value MUST
		// be at least twice the Part Target Duration.  Its value SHOULD be
		// at least three times the Part Target Duration.
		cnt += ",PART-HOLD-BACK=" + strconv.FormatFloat((part.duration*2).Seconds(), 'f', -1, 64)

		// Indicates that the Server can produce Playlist Delta Updates in
		// response to the _HLS_skip Delivery Directive.  Its value is the
		// Skip Boundary, a decimal-floating-point number of seconds.  The
		// Skip Boundary MUST be at least six times the Target Duration.
		cnt += ",CAN-SKIP-UNTIL=" + strconv.FormatFloat(float64(targetDuration), 'f', -1, 64) + "\n"

		cnt += "#EXT-X-PART-INF:PART-TARGET=" + strconv.FormatFloat(part.duration.Seconds(), 'f', -1, 64) + "\n"
	}

	cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(p.segmentDeleteCount), 10) + "\n"
	cnt += "#EXT-X-INDEPENDENT-SEGMENTS" + "\n"
	cnt += "#EXT-X-MAP:URI=\"init.mp4\"\n"
	cnt += "\n"

	for _, segment := range p.segments {
		cnt += "#EXT-X-PROGRAM-DATE-TIME:" + segment.startTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n"

		if p.lowLatency {
			for i, part := range segment.parts {
				cnt += "#EXT-X-PART:DURATION=" + strconv.FormatFloat(part.duration.Seconds(), 'f', -1, 64) +
					",URI=\"" + part.name() + ".mp4\""
				if i == 0 {
					cnt += ",INDEPENDENT=YES"
				}
				cnt += "\n"
			}
		}

		cnt += "#EXTINF:" + strconv.FormatFloat(segment.duration.Seconds(), 'f', -1, 64) + ",\n" +
			segment.name() + ".mp4\n"
	}

	if p.lowLatency {
		for i, part := range p.nextSegmentParts {
			cnt += "#EXT-X-PART:DURATION=" + strconv.FormatFloat(part.duration.Seconds(), 'f', -1, 64) +
				",URI=\"" + part.name() + ".mp4\""
			if i == 0 {
				cnt += ",INDEPENDENT=YES"
			}
			cnt += "\n"
		}

		// preload hint must always be present
		// otherwise hls.js goes into a loop
		cnt += "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"" + fmp4PartName(p.nextPartID) + ".mp4\"\n"
	}

	return bytes.NewReader([]byte(cnt))
}

func (p *muxerVariantFMP4Playlist) segmentReader(fname string) *MuxerFileResponse {
	switch {
	case fname == "init.mp4":
		p.mutex.Lock()
		defer p.mutex.Unlock()

		byts, err := mp4InitGenerate(p.videoTrack, p.audioTrack)
		if err != nil {
			return &MuxerFileResponse{Status: http.StatusInternalServerError}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: bytes.NewReader(byts),
		}

	case strings.HasPrefix(fname, "seg"):
		base := strings.TrimSuffix(fname, ".mp4")

		p.mutex.Lock()
		segment, ok := p.segmentsByName[base]
		p.mutex.Unlock()

		if !ok {
			return &MuxerFileResponse{Status: http.StatusNotFound}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: segment.reader(),
		}

	case strings.HasPrefix(fname, "part"):
		base := strings.TrimSuffix(fname, ".mp4")

		p.mutex.Lock()
		part, ok := p.partsByName[base]
		nextPartID := p.nextPartID
		p.mutex.Unlock()

		if ok {
			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: part.reader(),
			}
		}

		if fname == fmp4PartName(p.nextPartID) {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			for {
				if p.closed {
					break
				}

				if p.nextPartID > nextPartID {
					break
				}

				p.cond.Wait()
			}

			if p.closed {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: p.partsByName[fmp4PartName(nextPartID)].reader(),
			}
		}

		return &MuxerFileResponse{Status: http.StatusNotFound}

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerVariantFMP4Playlist) onSegmentFinalized(segment *muxerVariantFMP4Segment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.segmentsByName[segment.name()] = segment
		p.segments = append(p.segments, segment)
		p.nextSegmentID = segment.id + 1
		p.nextSegmentParts = p.nextSegmentParts[:0]

		if len(p.segments) > p.segmentCount {
			for _, part := range p.segments[0].parts {
				delete(p.partsByName, part.name())
			}
			p.parts = p.parts[len(p.segments[0].parts):]

			delete(p.segmentsByName, p.segments[0].name())
			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}

func (p *muxerVariantFMP4Playlist) onPartFinalized(part *muxerVariantFMP4Part) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.partsByName[part.name()] = part
		p.parts = append(p.parts, part)
		p.nextSegmentParts = append(p.nextSegmentParts, part)
		p.nextPartID = part.id + 1
	}()

	p.cond.Broadcast()
}
