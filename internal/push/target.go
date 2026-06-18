package push

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	retryPause = 5 * time.Second
)

type targetReader struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (r *targetReader) Close() {
	r.once.Do(r.cancel)
}

func (*targetReader) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHidden,
		ID:   "",
	}
}

// Target is a push target.
type Target struct {
	URL               string
	Source            defs.APIPushTargetSource
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	PathName          string
	Matches           []string
	PathManager       PathManager
	Parent            logger.Writer

	ctx       context.Context
	ctxCancel func()
	done      chan struct{}

	uuid    uuid.UUID
	created time.Time

	mutex     sync.RWMutex
	state     defs.APIPushTargetState
	lastError string
}

// Initialize initializes Target.
func (t *Target) Initialize() {
	t.ctx, t.ctxCancel = context.WithCancel(context.Background())
	t.done = make(chan struct{})
	t.uuid = uuid.New()
	t.created = time.Now()
	t.setState(defs.APIPushTargetStateConnecting, "")

	go t.run()
}

// ID returns the ID.
func (t *Target) ID() uuid.UUID {
	return t.uuid
}

// Close closes Target.
func (t *Target) Close() {
	t.ctxCancel()
	<-t.done
}

// Log implements logger.Writer.
func (t *Target) Log(level logger.Level, format string, args ...any) {
	t.Parent.Log(level, "[target "+t.uuid.String()+"] "+format, args...)
}

func (t *Target) setState(state defs.APIPushTargetState, lastError string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.state = state
	t.lastError = lastError
}

// APIItem returns an API item.
func (t *Target) APIItem() defs.APIPushTarget {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return defs.APIPushTarget{
		ID:        t.uuid,
		Created:   t.created,
		URL:       t.URL,
		Source:    t.Source,
		State:     t.state,
		LastError: t.lastError,
	}
}

func resolveURL(rawURL string, pathName string, matches []string) string {
	out := strings.ReplaceAll(rawURL, "$MTX_PATH", pathName)
	out = strings.ReplaceAll(out, "$path", pathName)

	if len(matches) > 1 {
		for i, ma := range matches[1:] {
			out = strings.ReplaceAll(out, fmt.Sprintf("$G%d", i+1), ma)
		}
	}

	return out
}

func (t *Target) run() {
	defer close(t.done)

	for {
		err := t.runOnce()
		if t.ctx.Err() != nil {
			return
		}

		t.setState(defs.APIPushTargetStateError, err.Error())
		t.Log(logger.Error, err.Error())

		timer := time.NewTimer(retryPause)
		select {
		case <-timer.C:
		case <-t.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (t *Target) runOnce() error {
	t.setState(defs.APIPushTargetStateConnecting, "")

	readerCtx, readerCancel := context.WithCancel(t.ctx)
	defer readerCancel()

	reader := &targetReader{
		cancel: readerCancel,
	}

	res, err := t.PathManager.AddReader(defs.PathAddReaderReq{
		Author: reader,
		AccessRequest: defs.PathAccessRequest{
			Name:      t.PathName,
			SkipAuth:  true,
			UserAgent: "mediamtx-push-target",
		},
		Cancel: readerCtx.Done(),
	})
	if err != nil {
		return err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: reader})

	rawURL := resolveURL(t.URL, t.PathName, t.Matches)
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	t.setState(defs.APIPushTargetStatePushing, "")
	t.Log(logger.Info, "pushing to '%s'", rawURL)

	switch u.Scheme {
	case "rtmp", "rtmps":
		err = t.runRTMP(readerCtx, u, res.Stream)

	case "rtsp", "rtsps":
		err = t.runRTSP(readerCtx, rawURL, res.Stream)

	case "srt":
		err = t.runSRT(readerCtx, rawURL, res.Stream)

	default:
		err = fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	if err != nil {
		return err
	}

	return fmt.Errorf("terminated")
}

func fourCCToString(c message.FourCC) string {
	return string([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
}

func rtmpFourCCList(desc *description.Session) amf0.StrictArray {
	var videoCount int
	var audioCount int
	var enhanced bool

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma.(type) {
			case *format.AV1, *format.VP9, *format.H265, *format.Opus, *format.AC3:
				enhanced = true

			case *format.Generic:
				enhanced = true

			default:
				switch media.Type {
				case description.MediaTypeVideo:
					videoCount++
				case description.MediaTypeAudio:
					audioCount++
				}
			}
		}
	}

	if !enhanced && videoCount <= 1 && audioCount <= 1 {
		return nil
	}

	return amf0.StrictArray{
		fourCCToString(message.FourCCAV1),
		fourCCToString(message.FourCCVP9),
		fourCCToString(message.FourCCHEVC),
		fourCCToString(message.FourCCAVC),
		fourCCToString(message.FourCCOpus),
		fourCCToString(message.FourCCFLAC),
		fourCCToString(message.FourCCAC3),
		fourCCToString(message.FourCCMP4A),
		fourCCToString(message.FourCCMP3),
	}
}

func (t *Target) waitReader(ctx context.Context, r *stream.Reader, closeConn func()) error {
	select {
	case err := <-r.Error():
		return err

	case <-ctx.Done():
		closeConn()
		return fmt.Errorf("terminated")
	}
}

func rtmpURLWithDefaultPort(u *url.URL) *url.URL {
	if u.Port() != "" {
		return u
	}

	du := *u
	if u.Scheme == "rtmp" {
		du.Host = net.JoinHostPort(u.Hostname(), "1935")
	} else {
		du.Host = net.JoinHostPort(u.Hostname(), "1936")
	}
	return &du
}

func (t *Target) runRTMP(ctx context.Context, u *url.URL, strm *stream.Stream) error {
	var conn interface {
		gortmplib.Conn
		Close()
		Initialize(context.Context) error
		NetConn() net.Conn
	}
	if u.User != nil {
		conn = &gortmplib.Client{
			URL:     rtmpURLWithDefaultPort(u),
			Publish: true,
		}
	} else {
		conn = &rtmpPublishClient{
			URL: rtmpURLWithDefaultPort(u),
		}
	}

	err := conn.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("connect RTMP target: %w", err)
	}
	defer conn.Close()

	r := &stream.Reader{Parent: t}
	outDesc := strm.OutDescCopy()

	err = rtmp.FromStream(
		strm.OrigDesc,
		outDesc,
		r,
		conn,
		conn.NetConn(),
		time.Duration(t.WriteTimeout),
		rtmpFourCCList(outDesc))
	if err != nil {
		return fmt.Errorf("initialize RTMP target writer: %w", err)
	}

	conn.NetConn().SetReadDeadline(time.Time{})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	return t.waitReader(ctx, r, conn.Close)
}

func (t *Target) runRTSP(ctx context.Context, rawURL string, strm *stream.Stream) error {
	desc := strm.OutDescCopy()

	dialer := &net.Dialer{}
	client := &gortsplib.Client{
		ReadTimeout:  time.Duration(t.ReadTimeout),
		WriteTimeout: time.Duration(t.WriteTimeout),
		DialContext: func(_ context.Context, network string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}

	err := client.StartRecording(rawURL, desc)
	if err != nil {
		return err
	}
	defer client.Close()

	r := &stream.Reader{Parent: t}

	for i, media := range strm.OrigDesc.Medias {
		outMedia := desc.Medias[i]

		for _, forma := range media.Formats {
			r.OnData(media, forma, func(u *unit.Unit) error {
				for _, pkt := range u.RTPPackets {
					writeErr := client.WritePacketRTPWithNTP(outMedia, pkt, u.NTP)
					if writeErr != nil {
						return writeErr
					}
				}
				return nil
			})
		}
	}

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	return t.waitReader(ctx, r, client.Close)
}

func srtMaxPayloadSize(u int) int {
	return ((u - 16) / 188) * 188
}

func (t *Target) runSRT(ctx context.Context, rawURL string, strm *stream.Stream) error {
	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL(rawURL)
	if err != nil {
		return err
	}

	udpMaxPayloadSize := t.UDPMaxPayloadSize
	if udpMaxPayloadSize == 0 {
		udpMaxPayloadSize = 1472
	}
	conf.PayloadSize = uint32(srtMaxPayloadSize(udpMaxPayloadSize))

	err = conf.Validate()
	if err != nil {
		return err
	}

	sconn, err := srt.Dial("srt", address, conf)
	if err != nil {
		return err
	}
	defer sconn.Close()

	r := &stream.Reader{Parent: t}
	bw := bufio.NewWriterSize(sconn, int(conf.PayloadSize))

	err = mpegts.FromStream(strm.OrigDesc, r, bw, sconn, time.Duration(t.WriteTimeout))
	if err != nil {
		return err
	}

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	return t.waitReader(ctx, r, func() {
		sconn.Close()
	})
}
