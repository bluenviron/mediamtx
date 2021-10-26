package hls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	gopath "path"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/asticode/go-astits"
	"github.com/grafov/m3u8"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	clientMinDownloadPause             = 5 * time.Second
	clientQueueSize                    = 100
	clientMinSegmentsBeforeDownloading = 2
)

func clientURLAbsolute(base *url.URL, relative string) (*url.URL, error) {
	u, err := url.Parse(relative)
	if err != nil {
		return nil, err
	}

	if !u.IsAbs() {
		u = &url.URL{
			Scheme: base.Scheme,
			User:   base.User,
			Host:   base.Host,
			Path:   gopath.Join(gopath.Dir(base.Path), relative),
		}
	}

	return u, nil
}

type clientSegmentQueue struct {
	mutex   sync.Mutex
	queue   [][]byte
	didPush chan struct{}
	didPull chan struct{}
}

func newClientSegmentQueue() *clientSegmentQueue {
	return &clientSegmentQueue{
		didPush: make(chan struct{}),
		didPull: make(chan struct{}),
	}
}

func (q *clientSegmentQueue) push(seg []byte) {
	q.mutex.Lock()

	queueWasEmpty := (len(q.queue) == 0)
	q.queue = append(q.queue, seg)

	if queueWasEmpty {
		close(q.didPush)
		q.didPush = make(chan struct{})
	}

	q.mutex.Unlock()
}

func (q *clientSegmentQueue) waitUntilSizeIsBelow(ctx context.Context, n int) {
	q.mutex.Lock()

	for len(q.queue) > n {
		q.mutex.Unlock()

		select {
		case <-q.didPull:
		case <-ctx.Done():
			return
		}

		q.mutex.Lock()
	}

	q.mutex.Unlock()
}

func (q *clientSegmentQueue) waitAndPull(ctx context.Context) ([]byte, error) {
	q.mutex.Lock()

	for len(q.queue) == 0 {
		q.mutex.Unlock()

		select {
		case <-q.didPush:
		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}

		q.mutex.Lock()
	}

	var seg []byte
	seg, q.queue = q.queue[0], q.queue[1:]

	close(q.didPull)
	q.didPull = make(chan struct{})

	q.mutex.Unlock()
	return seg, nil
}

type clientAllocateProcsReq struct {
	res chan struct{}
}

type clientVideoProcessorData struct {
	data []byte
	pts  time.Duration
	dts  time.Duration
}

type clientVideoProcessor struct {
	ctx     context.Context
	onTrack func(*gortsplib.Track) error
	onFrame func([]byte)

	queue         chan clientVideoProcessorData
	sps           []byte
	pps           []byte
	encoder       *rtph264.Encoder
	clockStartRTC time.Time
}

func newClientVideoProcessor(
	ctx context.Context,
	onTrack func(*gortsplib.Track) error,
	onFrame func([]byte),
) *clientVideoProcessor {
	p := &clientVideoProcessor{
		ctx:     ctx,
		onTrack: onTrack,
		onFrame: onFrame,
		queue:   make(chan clientVideoProcessorData, clientQueueSize),
	}

	return p
}

func (p *clientVideoProcessor) run() error {
	for {
		select {
		case item := <-p.queue:
			err := p.doProcess(item.data, item.pts, item.dts)
			if err != nil {
				return err
			}

		case <-p.ctx.Done():
			return nil
		}
	}
}

func (p *clientVideoProcessor) doProcess(
	data []byte,
	pts time.Duration,
	dts time.Duration) error {
	elapsed := time.Since(p.clockStartRTC)
	if dts > elapsed {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(dts - elapsed):
		}
	}

	nalus, err := h264.DecodeAnnexB(data)
	if err != nil {
		return err
	}

	outNALUs := make([][]byte, 0, len(nalus))

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if p.sps == nil {
				p.sps = append([]byte(nil), nalu...)

				if p.encoder == nil && p.pps != nil {
					err := p.initializeTrack()
					if err != nil {
						return err
					}
				}
			}

			// remove since it's not needed
			continue

		case h264.NALUTypePPS:
			if p.pps == nil {
				p.pps = append([]byte(nil), nalu...)

				if p.encoder == nil && p.sps != nil {
					err := p.initializeTrack()
					if err != nil {
						return err
					}
				}
			}

			// remove since it's not needed
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it's not needed
			continue
		}

		outNALUs = append(outNALUs, nalu)
	}

	if len(outNALUs) == 0 {
		return nil
	}

	if p.encoder == nil {
		return nil
	}

	pkts, err := p.encoder.Encode(outNALUs, pts)
	if err != nil {
		return fmt.Errorf("error while encoding H264: %v", err)
	}

	bytss := make([][]byte, len(pkts))
	for i, pkt := range pkts {
		byts, err := pkt.Marshal()
		if err != nil {
			return fmt.Errorf("error while encoding H264: %v", err)
		}
		bytss[i] = byts
	}

	for _, byts := range bytss {
		p.onFrame(byts)
	}

	return nil
}

func (p *clientVideoProcessor) process(
	data []byte,
	pts time.Duration,
	dts time.Duration) {
	p.queue <- clientVideoProcessorData{data, pts, dts}
}

func (p *clientVideoProcessor) initializeTrack() error {
	track, err := gortsplib.NewTrackH264(96, &gortsplib.TrackConfigH264{SPS: p.sps, PPS: p.pps})
	if err != nil {
		return err
	}

	p.encoder = rtph264.NewEncoder(96, nil, nil, nil)

	return p.onTrack(track)
}

type clientAudioProcessorData struct {
	data []byte
	pts  time.Duration
}

type clientAudioProcessor struct {
	ctx     context.Context
	onTrack func(*gortsplib.Track) error
	onFrame func([]byte)

	queue         chan clientAudioProcessorData
	conf          *gortsplib.TrackConfigAAC
	encoder       *rtpaac.Encoder
	clockStartRTC time.Time
}

func newClientAudioProcessor(
	ctx context.Context,
	onTrack func(*gortsplib.Track) error,
	onFrame func([]byte),
) *clientAudioProcessor {
	p := &clientAudioProcessor{
		ctx:     ctx,
		onTrack: onTrack,
		onFrame: onFrame,
		queue:   make(chan clientAudioProcessorData, clientQueueSize),
	}

	return p
}

func (p *clientAudioProcessor) run() error {
	for {
		select {
		case item := <-p.queue:
			err := p.doProcess(item.data, item.pts)
			if err != nil {
				return err
			}

		case <-p.ctx.Done():
			return nil
		}
	}
}

func (p *clientAudioProcessor) doProcess(
	data []byte,
	pts time.Duration) error {
	adtsPkts, err := aac.DecodeADTS(data)
	if err != nil {
		return err
	}

	aus := make([][]byte, 0, len(adtsPkts))

	pktPts := pts

	now := time.Now()

	for _, pkt := range adtsPkts {
		elapsed := now.Sub(p.clockStartRTC)

		if pktPts > elapsed {
			select {
			case <-p.ctx.Done():
				return fmt.Errorf("terminated")
			case <-time.After(pktPts - elapsed):
			}
		}

		if p.conf == nil {
			p.conf = &gortsplib.TrackConfigAAC{
				Type:         pkt.Type,
				SampleRate:   pkt.SampleRate,
				ChannelCount: pkt.ChannelCount,
			}

			if p.encoder == nil {
				err := p.initializeTrack()
				if err != nil {
					return err
				}
			}
		}

		aus = append(aus, pkt.AU)
		pktPts += 1000 * time.Second / time.Duration(pkt.SampleRate)
	}

	if p.encoder == nil {
		return nil
	}

	pkts, err := p.encoder.Encode(aus, pts)
	if err != nil {
		return fmt.Errorf("error while encoding AAC: %v", err)
	}

	bytss := make([][]byte, len(pkts))
	for i, pkt := range pkts {
		byts, err := pkt.Marshal()
		if err != nil {
			return fmt.Errorf("error while encoding AAC: %v", err)
		}
		bytss[i] = byts
	}

	for _, byts := range bytss {
		p.onFrame(byts)
	}

	return nil
}

func (p *clientAudioProcessor) process(
	data []byte,
	pts time.Duration) {
	select {
	case p.queue <- clientAudioProcessorData{data, pts}:
	case <-p.ctx.Done():
	}
}

func (p *clientAudioProcessor) initializeTrack() error {
	track, err := gortsplib.NewTrackAAC(97, p.conf)
	if err != nil {
		return err
	}

	p.encoder = rtpaac.NewEncoder(97, p.conf.SampleRate, nil, nil, nil)

	return p.onTrack(track)
}

// ClientParent is the parent of a Client.
type ClientParent interface {
	Log(level logger.Level, format string, args ...interface{})
}

// Client is a HLS client.
type Client struct {
	ur       string
	onTracks func(*gortsplib.Track, *gortsplib.Track) error
	onFrame  func(bool, []byte)
	parent   ClientParent

	ctx                   context.Context
	ctxCancel             func()
	httpClient            *http.Client
	urlParsed             *url.URL
	lastDownloadTime      time.Time
	downloadedSegmentURIs []string
	segmentQueue          *clientSegmentQueue
	pmtDownloaded         bool
	clockInitialized      bool
	clockStartPTS         time.Duration

	videoPID  *uint16
	audioPID  *uint16
	videoProc *clientVideoProcessor
	audioProc *clientAudioProcessor

	tracksMutex sync.RWMutex
	videoTrack  *gortsplib.Track
	audioTrack  *gortsplib.Track

	// in
	allocateProcs chan clientAllocateProcsReq

	// out
	outErr chan error
}

// NewClient allocates a Client.
func NewClient(
	ur string,
	fingerprint string,
	onTracks func(*gortsplib.Track, *gortsplib.Track) error,
	onFrame func(bool, []byte),
	parent ClientParent,
) *Client {
	ctx, ctxCancel := context.WithCancel(context.Background())

	tlsConfig := &tls.Config{}

	if fingerprint != "" {
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			h := sha256.New()
			h.Write(cs.PeerCertificates[0].Raw)
			hstr := hex.EncodeToString(h.Sum(nil))
			fingerprintLower := strings.ToLower(fingerprint)

			if hstr != fingerprintLower {
				return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
					fingerprintLower, hstr)
			}

			return nil
		}
	}

	c := &Client{
		ur:        ur,
		onTracks:  onTracks,
		onFrame:   onFrame,
		parent:    parent,
		ctx:       ctx,
		ctxCancel: ctxCancel,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		segmentQueue:  newClientSegmentQueue(),
		allocateProcs: make(chan clientAllocateProcsReq),
		outErr:        make(chan error, 1),
	}

	go c.run()

	return c
}

func (c *Client) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, format, args...)
}

// Close closes all the Client resources.
func (c *Client) Close() {
	c.ctxCancel()
}

// Wait waits for any error of the Client.
func (c *Client) Wait() chan error {
	return c.outErr
}

func (c *Client) run() {
	c.outErr <- c.runInner()
}

func (c *Client) runInner() error {
	innerCtx, innerCtxCancel := context.WithCancel(context.Background())

	errChan := make(chan error)

	go func() { errChan <- c.runDownloader(innerCtx) }()
	go func() { errChan <- c.runProcessor(innerCtx) }()

	for {
		select {
		case req := <-c.allocateProcs:
			if c.videoPID != nil {
				c.videoProc = newClientVideoProcessor(
					innerCtx,
					c.onVideoTrack,
					c.onVideoFrame)

				go func() { errChan <- c.videoProc.run() }()
			}

			if c.audioPID != nil {
				c.audioProc = newClientAudioProcessor(
					innerCtx,
					c.onAudioTrack,
					c.onAudioFrame)

				go func() { errChan <- c.audioProc.run() }()
			}

			close(req.res)

		case err := <-errChan:
			innerCtxCancel()

			<-errChan
			if c.videoProc != nil {
				<-errChan
			}
			if c.audioProc != nil {
				<-errChan
			}

			return err

		case <-c.ctx.Done():
			innerCtxCancel()

			<-errChan
			<-errChan
			if c.videoProc != nil {
				<-errChan
			}
			if c.audioProc != nil {
				<-errChan
			}

			return fmt.Errorf("terminated")
		}
	}
}

func (c *Client) runDownloader(innerCtx context.Context) error {
	for {
		c.segmentQueue.waitUntilSizeIsBelow(innerCtx, clientMinSegmentsBeforeDownloading)

		_, err := c.fillSegmentQueue(innerCtx)
		if err != nil {
			return err
		}
	}
}

func (c *Client) fillSegmentQueue(innerCtx context.Context) (bool, error) {
	minTime := c.lastDownloadTime.Add(clientMinDownloadPause)
	now := time.Now()
	if now.Before(minTime) {
		select {
		case <-time.After(minTime.Sub(now)):
		case <-innerCtx.Done():
			return false, fmt.Errorf("terminated")
		}
	}

	c.lastDownloadTime = now

	pl, err := func() (*m3u8.MediaPlaylist, error) {
		if c.urlParsed == nil {
			return c.downloadPrimaryPlaylist(innerCtx)
		}
		return c.downloadStreamPlaylist(innerCtx)
	}()
	if err != nil {
		return false, err
	}

	added := false

	for _, seg := range pl.Segments {
		if seg == nil {
			break
		}

		if !c.segmentWasDownloaded(seg.URI) {
			c.downloadedSegmentURIs = append(c.downloadedSegmentURIs, seg.URI)
			byts, err := c.downloadSegment(innerCtx, seg.URI)
			if err != nil {
				return false, err
			}

			c.segmentQueue.push(byts)
			added = true
		}
	}

	return added, nil
}

func (c *Client) segmentWasDownloaded(ur string) bool {
	for _, q := range c.downloadedSegmentURIs {
		if q == ur {
			return true
		}
	}
	return false
}

func (c *Client) downloadPrimaryPlaylist(innerCtx context.Context) (*m3u8.MediaPlaylist, error) {
	c.log(logger.Debug, "downloading primary playlist %s", c.ur)

	var err error
	c.urlParsed, err = url.Parse(c.ur)
	if err != nil {
		return nil, err
	}

	pl, err := c.downloadPlaylist(innerCtx)
	if err != nil {
		return nil, err
	}

	switch plt := pl.(type) {
	case *m3u8.MediaPlaylist:
		return plt, nil

	case *m3u8.MasterPlaylist:
		// choose the variant with the highest bandwidth
		var chosenVariant *m3u8.Variant
		for _, v := range plt.Variants {
			if chosenVariant == nil ||
				v.VariantParams.Bandwidth > chosenVariant.VariantParams.Bandwidth {
				chosenVariant = v
			}
		}

		if chosenVariant == nil {
			return nil, fmt.Errorf("no variants found")
		}

		u, err := clientURLAbsolute(c.urlParsed, chosenVariant.URI)
		if err != nil {
			return nil, err
		}

		c.urlParsed = u

		return c.downloadStreamPlaylist(innerCtx)

	default:
		return nil, fmt.Errorf("invalid playlist")
	}
}

func (c *Client) downloadStreamPlaylist(innerCtx context.Context) (*m3u8.MediaPlaylist, error) {
	c.log(logger.Debug, "downloading stream playlist %s", c.urlParsed.String())

	pl, err := c.downloadPlaylist(innerCtx)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (c *Client) downloadPlaylist(innerCtx context.Context) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(innerCtx, http.MethodGet, c.urlParsed.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	pl, _, err := m3u8.DecodeFrom(res.Body, true)
	if err != nil {
		return nil, err
	}

	return pl, nil
}

func (c *Client) downloadSegment(innerCtx context.Context, segmentURI string) ([]byte, error) {
	u, err := clientURLAbsolute(c.urlParsed, segmentURI)
	if err != nil {
		return nil, err
	}

	c.log(logger.Debug, "downloading segment %s", u)
	req, err := http.NewRequestWithContext(innerCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return byts, nil
}

func (c *Client) runProcessor(innerCtx context.Context) error {
	for {
		seg, err := c.segmentQueue.waitAndPull(innerCtx)
		if err != nil {
			return err
		}

		err = c.processSegment(innerCtx, seg)
		if err != nil {
			return err
		}
	}
}

func (c *Client) processSegment(innerCtx context.Context, byts []byte) error {
	dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

	// parse PMT
	if !c.pmtDownloaded {
		for {
			data, err := dem.NextData()
			if err != nil {
				if err == astits.ErrNoMorePackets {
					return nil
				}
				return err
			}

			if data.PMT != nil {
				c.pmtDownloaded = true

				for _, e := range data.PMT.ElementaryStreams {
					switch e.StreamType {
					case astits.StreamTypeH264Video:
						if c.videoPID != nil {
							return fmt.Errorf("multiple video/audio tracks are not supported")
						}

						v := e.ElementaryPID
						c.videoPID = &v

					case astits.StreamTypeAACAudio:
						if c.audioPID != nil {
							return fmt.Errorf("multiple video/audio tracks are not supported")
						}

						v := e.ElementaryPID
						c.audioPID = &v
					}
				}
				break
			}
		}

		if c.videoPID == nil && c.audioPID == nil {
			return fmt.Errorf("stream doesn't contain tracks with supported codecs (H264 or AAC)")
		}

		res := make(chan struct{})
		select {
		case c.allocateProcs <- clientAllocateProcsReq{res}:
			<-res
		case <-innerCtx.Done():
			return nil
		}
	}

	// process PES packets
	for {
		data, err := dem.NextData()
		if err != nil {
			if err == astits.ErrNoMorePackets {
				return nil
			}
			if strings.HasPrefix(err.Error(), "astits: parsing PES data failed") {
				continue
			}
			return err
		}

		if data.PES == nil {
			continue
		}

		if data.PES.Header.OptionalHeader == nil ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorNoPTSOrDTS ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorIsForbidden {
			return fmt.Errorf("PTS is missing")
		}

		pts := time.Duration(float64(data.PES.Header.OptionalHeader.PTS.Base) * float64(time.Second) / 90000)

		if !c.clockInitialized {
			c.clockInitialized = true
			c.clockStartPTS = pts
			now := time.Now()

			if c.videoPID != nil {
				c.videoProc.clockStartRTC = now
			}

			if c.audioPID != nil {
				c.audioProc.clockStartRTC = now
			}
		}

		if c.videoPID != nil && data.PID == *c.videoPID {
			var dts time.Duration
			if data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
				dts = time.Duration(float64(data.PES.Header.OptionalHeader.DTS.Base) * float64(time.Second) / 90000)
			} else {
				dts = pts
			}

			pts -= c.clockStartPTS
			dts -= c.clockStartPTS

			c.videoProc.process(data.PES.Data, pts, dts)

		} else if c.audioPID != nil && data.PID == *c.audioPID {
			pts -= c.clockStartPTS

			c.audioProc.process(data.PES.Data, pts)
		}
	}
}

func (c *Client) onVideoTrack(track *gortsplib.Track) error {
	c.tracksMutex.Lock()
	defer c.tracksMutex.Unlock()

	c.videoTrack = track

	if c.audioPID == nil || c.audioTrack != nil {
		return c.initializeTracks()
	}

	return nil
}

func (c *Client) onAudioTrack(track *gortsplib.Track) error {
	c.tracksMutex.Lock()
	defer c.tracksMutex.Unlock()

	c.audioTrack = track

	if c.videoPID == nil || c.videoTrack != nil {
		return c.initializeTracks()
	}

	return nil
}

func (c *Client) initializeTracks() error {
	return c.onTracks(c.videoTrack, c.audioTrack)
}

func (c *Client) onVideoFrame(payload []byte) {
	c.tracksMutex.RLock()
	defer c.tracksMutex.RUnlock()

	c.onFrame(true, payload)
}

func (c *Client) onAudioFrame(payload []byte) {
	c.tracksMutex.RLock()
	defer c.tracksMutex.RUnlock()

	c.onFrame(false, payload)
}
