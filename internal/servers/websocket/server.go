package websocket

import (
	"context"
	"net/http"
	"sync"
	"time"
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/servers/rtmp"
	"github.com/bluenviron/mediamtx/internal/servers/rtsp"
	"github.com/bluenviron/mediamtx/internal/servers/srt"
	"github.com/bluenviron/mediamtx/internal/servers/webrtc"
)

type Server struct {
	Address string
	Parent  logger.Writer

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	upgrader  websocket.Upgrader
	serveMux  *http.ServeMux
	server    *http.Server
	conns     map[*websocket.Conn]bool
	connMutex sync.Mutex
	
	HlsServer    *hls.Server
	RtmpServer   *rtmp.Server
	RtspServer   *rtsp.Server
	SrtServer    *srt.Server
	WebRTCServer *webrtc.Server
	WsInterval   int
}

func (s *Server) Initialize() error {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.serveMux = http.NewServeMux()
	s.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	s.conns = make(map[*websocket.Conn]bool)

	s.serveMux.HandleFunc("/ws", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.Address,
		Handler: s.serveMux,
	}

	go func() {
		s.Log(logger.Info, "listener opened on %s", s.Address)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Log(logger.Error, "server error: %v", err)
		}
	}()

	go s.sendStatsPeriodically()

	s.wg.Add(1)
	go s.run()

	return nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log(logger.Error, "failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	s.connMutex.Lock()
	s.conns[conn] = true
	s.connMutex.Unlock()

	s.Log(logger.Info, "new connection established")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.Log(logger.Error, "error reading message: %v", err)
			break
		}
		s.Log(logger.Info, "received message: %s", message)
	}

	s.connMutex.Lock()
	delete(s.conns, conn)
	s.connMutex.Unlock()
}

func (s *Server) run() {
	defer s.wg.Done()

	<-s.ctx.Done()
	s.Log(logger.Info, "listener is closing")

	s.connMutex.Lock()
	for conn := range s.conns {
		conn.Close()
	}
	s.conns = make(map[*websocket.Conn]bool)
	s.connMutex.Unlock()

	if err := s.server.Shutdown(s.ctx); err != nil {
		s.Log(logger.Error, "error closing listener: %v", err)
	}
}

func (s *Server) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	if s.Parent != nil {
		s.Parent.Log(level, "[WebSocket] "+format, args...)
	}
}

func (s *Server) sendStatsPeriodically() {
	ticker := time.NewTicker(time.Duration(s.WsInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := s.collectStats()
			s.sendStats(stats)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Server) collectStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Collect HLS stats
	if s.HlsServer != nil {
		hlsStats, err := s.HlsServer.APIMuxersList()
		if err != nil {
			s.Log(logger.Error, "error collecting HLS stats: %v", err)
		} else {
			stats["hls"] = hlsStats
		}
	}
	// Collect RTMP stats
	if s.RtmpServer != nil {
    		rtmpStats, err := s.RtmpServer.APIConnsList()
    		if err != nil {
        		s.Log(logger.Error, "error collecting RTMP stats: %v", err)
    		} else {
        		stats["rtmp"] = rtmpStats
    		}
	}
	// Collect RTSP stats
	if s.RtspServer != nil {
    		// Handle RTSP connections stats
    		rtspConnsStats, err := s.RtspServer.APIConnsList()
    		if err != nil {
        		s.Log(logger.Error, "error collecting RTSP conns stats: %v", err)
    		} else {
        		stats["rtsp_conns"] = rtspConnsStats
    		}

    		// Handle RTSP sessions stats
    		rtspSessionsStats, err := s.RtspServer.APISessionsList()
    		if err != nil {
        		s.Log(logger.Error, "error collecting RTSP sessions stats: %v", err)
   		} else {
       			stats["rtsp_sessions"] = rtspSessionsStats
    		}
	}
	// Collect SRT stats
	if s.SrtServer != nil {
    		srtStats, err := s.SrtServer.APIConnsList()
    		if err != nil {
        		s.Log(logger.Error, "error collecting SRT stats: %v", err)
    		} else {
        		stats["srt"] = srtStats
    		}
	}
	// Collect WebRTC stats
	if s.WebRTCServer != nil {
    		webRTCStats, err := s.WebRTCServer.APISessionsList()
    		if err != nil {
        		s.Log(logger.Error, "error collecting WebRTC stats: %v", err)
    		} else {
        		stats["webRTC"] = webRTCStats
    		}
	}
	
	return stats
}

func (s *Server) sendStats(stats map[string]interface{}) {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	for conn := range s.conns {
		statsJSON, err := json.Marshal(stats)
		if err != nil {
			s.Log(logger.Error, "error marshaling stats: %v", err)
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, statsJSON); err != nil {
			s.Log(logger.Error, "error sending stats: %v", err)
		}
	}
}
