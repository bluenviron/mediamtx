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
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
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
	return base.ResolveReference(u), nil
}

type clientAllocateProcsReq struct {
	res chan struct{}
}

// ClientLogger allows to receive log lines.
type ClientLogger interface {
	Log(level logger.Level, format string, args ...interface{})
}

// Client is a HLS client.
type Client struct {
	onTracks    func(*gortsplib.TrackH264, *gortsplib.TrackAAC) error
	onVideoData func(time.Duration, [][]byte)
	onAudioData func(time.Duration, [][]byte)
	logger      ClientLogger

	ctx                   context.Context
	ctxCancel             func()
	primaryPlaylistURL    *url.URL
	streamPlaylistURL     *url.URL
	httpClient            *http.Client
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
	videoTrack  *gortsplib.TrackH264
	audioTrack  *gortsplib.TrackAAC

	// in
	allocateProcs chan clientAllocateProcsReq

	// out
	outErr chan error
}

// NewClient allocates a Client.
func NewClient(
	primaryPlaylistURLStr string,
	fingerprint string,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackAAC) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, [][]byte),
	logger ClientLogger,
) (*Client, error) {
	primaryPlaylistURL, err := url.Parse(primaryPlaylistURLStr)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	var tlsConfig *tls.Config

	if fingerprint != "" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				h := sha256.New()
				h.Write(cs.PeerCertificates[0].Raw)
				hstr := hex.EncodeToString(h.Sum(nil))
				fingerprintLower := strings.ToLower(fingerprint)

				if hstr != fingerprintLower {
					return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
						fingerprintLower, hstr)
				}

				return nil
			},
		}
	}

	c := &Client{
		onTracks:           onTracks,
		onVideoData:        onVideoData,
		onAudioData:        onAudioData,
		logger:             logger,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		primaryPlaylistURL: primaryPlaylistURL,
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

	return c, nil
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
					c.onVideoProcessorTrack,
					c.onVideoProcessorData,
					c.logger)

				go func() { errChan <- c.videoProc.run() }()
			}

			if c.audioPID != nil {
				c.audioProc = newClientAudioProcessor(
					innerCtx,
					c.onAudioProcessorTrack,
					c.onAudioProcessorData)

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
		if c.streamPlaylistURL == nil {
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
	c.logger.Log(logger.Debug, "downloading primary playlist %s", c.primaryPlaylistURL)

	pl, err := c.downloadPlaylist(innerCtx, c.primaryPlaylistURL)
	if err != nil {
		return nil, err
	}

	switch plt := pl.(type) {
	case *m3u8.MediaPlaylist:
		c.streamPlaylistURL = c.primaryPlaylistURL
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

		u, err := clientURLAbsolute(c.primaryPlaylistURL, chosenVariant.URI)
		if err != nil {
			return nil, err
		}

		c.streamPlaylistURL = u

		return c.downloadStreamPlaylist(innerCtx)

	default:
		return nil, fmt.Errorf("invalid playlist")
	}
}

func (c *Client) downloadStreamPlaylist(innerCtx context.Context) (*m3u8.MediaPlaylist, error) {
	c.logger.Log(logger.Debug, "downloading stream playlist %s", c.streamPlaylistURL.String())

	pl, err := c.downloadPlaylist(innerCtx, c.streamPlaylistURL)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (c *Client) downloadPlaylist(innerCtx context.Context, ur *url.URL) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(innerCtx, http.MethodGet, ur.String(), nil)
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
	u, err := clientURLAbsolute(c.streamPlaylistURL, segmentURI)
	if err != nil {
		return nil, err
	}

	c.logger.Log(logger.Debug, "downloading segment %s", u)
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

func (c *Client) onVideoProcessorTrack(track *gortsplib.TrackH264) error {
	c.tracksMutex.Lock()
	defer c.tracksMutex.Unlock()

	c.videoTrack = track

	if c.audioPID == nil || c.audioTrack != nil {
		return c.onTracks(c.videoTrack, c.audioTrack)
	}

	return nil
}

func (c *Client) onVideoProcessorData(pts time.Duration, nalus [][]byte) {
	c.tracksMutex.RLock()
	defer c.tracksMutex.RUnlock()
	c.onVideoData(pts, nalus)
}

func (c *Client) onAudioProcessorTrack(track *gortsplib.TrackAAC) error {
	c.tracksMutex.Lock()
	defer c.tracksMutex.Unlock()

	c.audioTrack = track

	if c.videoPID == nil || c.videoTrack != nil {
		return c.onTracks(c.videoTrack, c.audioTrack)
	}

	return nil
}

func (c *Client) onAudioProcessorData(pts time.Duration, aus [][]byte) {
	c.tracksMutex.RLock()
	defer c.tracksMutex.RUnlock()
	c.onAudioData(pts, aus)
}
