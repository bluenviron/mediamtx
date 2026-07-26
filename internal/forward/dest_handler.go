package forward

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	forwardrtmp "github.com/bluenviron/mediamtx/internal/forward/rtmp"
	forwardrtsp "github.com/bluenviron/mediamtx/internal/forward/rtsp"
	forwardsrt "github.com/bluenviron/mediamtx/internal/forward/srt"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const retryPause = 5 * time.Second

type destReader struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (*destReader) Log(_ logger.Level, _ string, _ ...any) {
}

func (r *destReader) Close() {
	r.once.Do(r.cancel)
}

func (*destReader) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHidden,
		ID:   "",
	}
}

// DestHandler manages a forward destination.
type DestHandler struct {
	Dest              string
	Source            defs.APIForwardSource
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

	mutex         sync.RWMutex
	state         defs.APIForwardState
	lastError     string
	outboundBytes uint64
	activeDest    Dest
}

// Initialize initializes DestHandler.
func (h *DestHandler) Initialize() {
	h.ctx, h.ctxCancel = context.WithCancel(context.Background())
	h.done = make(chan struct{})
	h.uuid = uuid.New()
	h.created = time.Now()
	h.setState(defs.APIForwardStateConnecting, "")

	go h.run()
}

// ID returns the ID.
func (h *DestHandler) ID() uuid.UUID {
	return h.uuid
}

// Close closes DestHandler.
func (h *DestHandler) Close() {
	h.ctxCancel()
	<-h.done
}

// CloseAsync closes DestHandler without waiting for its goroutine to return.
func (h *DestHandler) CloseAsync() {
	h.ctxCancel()
}

// Log implements logger.Writer.
func (h *DestHandler) Log(level logger.Level, format string, args ...any) {
	h.Parent.Log(level, "[dest "+h.uuid.String()+"] "+format, args...)
}

func (h *DestHandler) setState(state defs.APIForwardState, lastError string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.state = state
	h.lastError = lastError
}

func (h *DestHandler) setActiveDest(dest Dest) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.activeDest = dest
}

func (h *DestHandler) clearActiveDest() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.activeDest != nil {
		h.outboundBytes += h.activeDest.OutboundBytes()
		h.activeDest = nil
	}
}

func (h *DestHandler) outboundBytesLocked() uint64 {
	outboundBytes := h.outboundBytes
	if h.activeDest != nil {
		outboundBytes += h.activeDest.OutboundBytes()
	}
	return outboundBytes
}

func destProtocol(dest string) defs.APIForwardProtocol {
	u, err := url.Parse(dest)
	if err != nil {
		return ""
	}
	return defs.APIForwardProtocol(u.Scheme)
}

// APIItem returns an API item.
func (h *DestHandler) APIItem() defs.APIForward {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	outboundBytes := h.outboundBytesLocked()

	return defs.APIForward{
		ID:            h.uuid,
		Created:       h.created,
		Dest:          h.Dest,
		Protocol:      destProtocol(h.Dest),
		Source:        h.Source,
		State:         h.state,
		LastError:     h.lastError,
		OutboundBytes: outboundBytes,
		BytesSent:     outboundBytes,
	}
}

func resolveDest(dest string, pathName string, matches []string) string {
	out := strings.ReplaceAll(dest, "$MTX_PATH", pathName)
	out = strings.ReplaceAll(out, "$path", pathName)

	if len(matches) > 1 {
		for i, ma := range matches[1:] {
			out = strings.ReplaceAll(out, fmt.Sprintf("$G%d", i+1), ma)
		}
	}

	return out
}

func (h *DestHandler) run() {
	defer close(h.done)

	for {
		err := h.runOnce()
		if h.ctx.Err() != nil {
			return
		}

		h.setState(defs.APIForwardStateError, err.Error())
		h.Log(logger.Error, err.Error())

		timer := time.NewTimer(retryPause)
		select {
		case <-timer.C:
		case <-h.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (h *DestHandler) runOnce() error {
	h.setState(defs.APIForwardStateConnecting, "")

	readerCtx, readerCancel := context.WithCancel(h.ctx)
	defer readerCancel()

	reader := &destReader{
		cancel: readerCancel,
	}

	res, err := h.PathManager.AddReader(defs.PathAddReaderReq{
		Author: reader,
		AccessRequest: defs.PathAccessRequest{
			Name:      h.PathName,
			SkipAuth:  true,
			UserAgent: "mediamtx-forward",
		},
		Cancel: readerCtx.Done(),
	})
	if err != nil {
		return err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: reader})

	resolvedDest := resolveDest(h.Dest, h.PathName, h.Matches)
	u, err := url.Parse(resolvedDest)
	if err != nil {
		return err
	}

	var dest Dest

	switch u.Scheme {
	case "rtmp", "rtmps":
		dest = &forwardrtmp.Dest{
			URL:          u,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case "rtsp", "rtsps":
		dest = &forwardrtsp.Dest{
			Dest:         resolvedDest,
			ReadTimeout:  h.ReadTimeout,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case "srt":
		dest = &forwardsrt.Dest{
			Dest:              resolvedDest,
			WriteTimeout:      h.WriteTimeout,
			UDPMaxPayloadSize: h.UDPMaxPayloadSize,
			Parent:            h,
		}

	default:
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	h.setState(defs.APIForwardStateForwarding, "")
	h.Log(logger.Info, "forwarding to '%s'", resolvedDest)
	h.setActiveDest(dest)
	defer h.clearActiveDest()

	err = dest.Run(readerCtx, res.Stream)
	if err != nil {
		return err
	}

	return fmt.Errorf("terminated")
}
