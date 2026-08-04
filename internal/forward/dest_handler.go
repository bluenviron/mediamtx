package forward

import (
	"context"
	"encoding/hex"
	"errors"
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
	"github.com/bluenviron/mediamtx/internal/stream"
)

const retryPause = 5 * time.Second

var errTerminated = errors.New("terminated")

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

// DestHandler manages a forward destination.
type DestHandler struct {
	Pos               int
	Conf              conf.ForwardDest
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	PathName          string
	Matches           []string
	Parent            logger.Writer

	ctx       context.Context
	ctxCancel func()

	uuid          uuid.UUID
	created       time.Time
	mutex         sync.RWMutex
	state         defs.APIForwardDestState
	lastError     string
	outboundBytes uint64
	activeDest    Dest

	done chan struct{}
}

func (h *DestHandler) initialize() {
	h.uuid = uuid.New()
	h.created = time.Now()
	h.state = defs.APIForwardDestStateIdle
}

func (h *DestHandler) start(strm *stream.Stream) {
	h.Log(logger.Debug, "starting")
	h.ctx, h.ctxCancel = context.WithCancel(context.Background())
	h.done = make(chan struct{})
	go h.run(strm)
}

func (h *DestHandler) stop() {
	h.Log(logger.Debug, "stopping")
	h.ctxCancel()
	<-h.done
}

// ID returns the ID.
func (h *DestHandler) ID() uuid.UUID {
	return h.uuid
}

// Log implements logger.Writer.
func (h *DestHandler) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(h.uuid[:4])
	h.Parent.Log(level, "[dest %d %s] "+format, append([]any{h.Pos, id}, args...)...)
}

func (h *DestHandler) outboundBytesLocked() uint64 {
	outboundBytes := h.outboundBytes
	if h.activeDest != nil {
		outboundBytes += h.activeDest.OutboundBytes()
	}
	return outboundBytes
}

func destProtocol(dest string) defs.APIForwardDestProtocol {
	u, err := url.Parse(dest)
	if err != nil {
		return ""
	}
	return defs.APIForwardDestProtocol(u.Scheme)
}

func (h *DestHandler) run(strm *stream.Stream) {
	defer close(h.done)

	defer func() {
		h.mutex.Lock()
		h.state = defs.APIForwardDestStateIdle
		h.mutex.Unlock()
	}()

	for {
		h.mutex.Lock()
		h.state = defs.APIForwardDestStateForwarding
		h.lastError = ""
		h.mutex.Unlock()

		err := h.runOnce(strm)
		if errors.Is(err, errTerminated) {
			return
		}

		h.mutex.Lock()
		h.state = defs.APIForwardDestStateError
		h.lastError = err.Error()
		h.mutex.Unlock()

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

func (h *DestHandler) runOnce(strm *stream.Stream) error {
	resolvedDest := resolveDest(h.Conf.Dest, h.PathName, h.Matches)
	u, err := url.Parse(resolvedDest)
	if err != nil {
		return err
	}

	var dest Dest

	switch u.Scheme {
	case "rtmp", "rtmps":
		dest = &forwardrtmp.Dest{
			Stream:       strm,
			URL:          u,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case "rtsp", "rtsps":
		dest = &forwardrtsp.Dest{
			Stream:       strm,
			Dest:         resolvedDest,
			ReadTimeout:  h.ReadTimeout,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case "srt":
		dest = &forwardsrt.Dest{
			Stream:            strm,
			Dest:              resolvedDest,
			WriteTimeout:      h.WriteTimeout,
			UDPMaxPayloadSize: h.UDPMaxPayloadSize,
			Parent:            h,
		}

	default:
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	h.Log(logger.Info, "forwarding to '%s'", resolvedDest)

	h.mutex.Lock()
	h.activeDest = dest
	h.mutex.Unlock()

	defer func() {
		h.mutex.Lock()
		h.outboundBytes += h.activeDest.OutboundBytes()
		h.activeDest = nil
		h.mutex.Unlock()
	}()

	destCtx, destCtxCancel := context.WithCancel(context.Background())

	errChan := make(chan error)
	go func() {
		errChan <- dest.Run(destCtx)
	}()

	select {
	case err = <-errChan:
		destCtxCancel()
		return err

	case <-h.ctx.Done():
		destCtxCancel()
		<-errChan
		return errTerminated
	}
}

// APIItem returns an API item.
func (h *DestHandler) APIItem() defs.APIForwardDest {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	outboundBytes := h.outboundBytesLocked()

	return defs.APIForwardDest{
		ID:            h.uuid,
		Pos:           h.Pos,
		Created:       h.created,
		Conf:          h.Conf,
		Protocol:      destProtocol(h.Conf.Dest),
		State:         h.state,
		LastError:     h.lastError,
		OutboundBytes: outboundBytes,
	}
}
