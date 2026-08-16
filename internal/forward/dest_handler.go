package forward

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	forwardrtmp "github.com/bluenviron/mediamtx/internal/forward/rtmp"
	forwardrtsp "github.com/bluenviron/mediamtx/internal/forward/rtsp"
	forwardsrt "github.com/bluenviron/mediamtx/internal/forward/srt"
	forwardwebrtc "github.com/bluenviron/mediamtx/internal/forward/webrtc"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const retryPause = 5 * time.Second

var errTerminated = errors.New("terminated")

func sanitizeDestURL(dest string) string {
	u, err := url.Parse(dest)
	if err != nil {
		return dest
	}
	u.User = nil
	u.Fragment = ""
	return u.String()
}

func resolveDest(dest string, pathName string, matches []string) string {
	out := strings.ReplaceAll(dest, "$MTX_PATH", pathName)

	for i := len(matches) - 1; i >= 1; i-- {
		out = strings.ReplaceAll(out, "$G"+strconv.FormatInt(int64(i), 10), matches[i])
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
	protocol      defs.APIForwardDestProtocol
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
	h.protocol = destProtocol(h.Conf.Dest)
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
	h.Parent.Log(level, "[%s dest %d %s] "+format,
		append([]any{strings.ToUpper(string(h.protocol)), h.Pos, id}, args...)...)
}

func (h *DestHandler) outboundBytesLocked() uint64 {
	outboundBytes := h.outboundBytes
	if h.activeDest != nil {
		outboundBytes += h.activeDest.OutboundBytes()
	}
	return outboundBytes
}

func destProtocol(dest string) defs.APIForwardDestProtocol {
	switch {
	case strings.HasPrefix(dest, "rtmp://"):
		return defs.APIForwardDestProtocolRTMP

	case strings.HasPrefix(dest, "rtmps://"):
		return defs.APIForwardDestProtocolRTMPS

	case strings.HasPrefix(dest, "rtsp://"):
		return defs.APIForwardDestProtocolRTSP

	case strings.HasPrefix(dest, "rtsps://"):
		return defs.APIForwardDestProtocolRTSPS

	case strings.HasPrefix(dest, "srt://"):
		return defs.APIForwardDestProtocolSRT

	case strings.HasPrefix(dest, "whip://"):
		return defs.APIForwardDestProtocolWHIP

	case strings.HasPrefix(dest, "whips://"):
		return defs.APIForwardDestProtocolWHIPS

	default:
		panic("should not happen")
	}
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

	var dest Dest

	switch h.protocol {
	case defs.APIForwardDestProtocolRTMP, defs.APIForwardDestProtocolRTMPS:
		dest = &forwardrtmp.Dest{
			Stream:       strm,
			Dest:         resolvedDest,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case defs.APIForwardDestProtocolRTSP, defs.APIForwardDestProtocolRTSPS:
		dest = &forwardrtsp.Dest{
			Stream:       strm,
			Dest:         resolvedDest,
			ReadTimeout:  h.ReadTimeout,
			WriteTimeout: h.WriteTimeout,
			Parent:       h,
		}

	case defs.APIForwardDestProtocolSRT:
		dest = &forwardsrt.Dest{
			Stream:            strm,
			Dest:              resolvedDest,
			WriteTimeout:      h.WriteTimeout,
			UDPMaxPayloadSize: h.UDPMaxPayloadSize,
			Parent:            h,
		}

	case defs.APIForwardDestProtocolWHIP, defs.APIForwardDestProtocolWHIPS:
		dest = &forwardwebrtc.Dest{
			Stream:          strm,
			Dest:            resolvedDest,
			ReadTimeout:     h.ReadTimeout,
			WhipBearerToken: h.Conf.WhipBearerToken,
			Parent:          h,
		}

	default:
		panic("should not happen")
	}

	h.Log(logger.Info, "forwarding to '%s'", sanitizeDestURL(resolvedDest))

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
	case err := <-errChan:
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
		Protocol:      h.protocol,
		State:         h.state,
		LastError:     h.lastError,
		OutboundBytes: outboundBytes,
	}
}
