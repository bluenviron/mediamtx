// Package playback contains the playback server.
package playback

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/gin-gonic/gin"
)

type serverAuthManager interface {
	Authenticate(req *auth.Request) (string, *auth.Error)
}

// Server is the playback server.
type Server struct {
	Address        string
	DumpPackets    bool
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigins   []string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	PathConfs      map[string]*conf.Path
	AuthManager    serverAuthManager
	Parent         logger.Writer

	httpServer *httpp.Server
	mutex      sync.RWMutex
}

// Initialize initializes Server.
func (s *Server) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(s.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(s.middlewarePreflightRequests)

	router.GET("/list", s.onList)
	router.GET("/get", s.onGet)

	s.httpServer = &httpp.Server{
		Address:           s.Address,
		AllowOrigins:      s.AllowOrigins,
		DumpPackets:       s.DumpPackets,
		DumpPacketsPrefix: "playback_server_conn",
		ReadTimeout:       time.Duration(s.ReadTimeout),
		WriteTimeout:      time.Duration(s.WriteTimeout),
		Encryption:        s.Encryption,
		ServerCert:        s.ServerCert,
		ServerKey:         s.ServerKey,
		Handler:           router,
		Parent:            s,
	}
	err := s.httpServer.Initialize()
	if err != nil {
		return err
	}

	str := "listener opened on " + s.Address
	if !s.Encryption {
		str += " (TCP/HTTP)"
	} else {
		str += " (TCP/HTTPS)"
	}
	s.Log(logger.Info, str)

	return nil
}

// Close closes Server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")
	s.httpServer.Close()
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[playback] "+format, args...)
}

// ReloadPathConfs is called by core.Core.
func (s *Server) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.PathConfs = pathConfs
}

func (s *Server) writeError(ctx *gin.Context, status int, err error) {
	// show error in logs
	s.Log(logger.Error, err.Error())

	// add error to response
	ctx.AbortWithStatusJSON(status, &defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  err.Error(),
	})
}

func (s *Server) writeErrorNoLog(ctx *gin.Context, status int, err error) {
	ctx.AbortWithStatusJSON(status, &defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  err.Error(),
	})
}

func (s *Server) safeFindPathConf(name string) (*conf.Path, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(s.PathConfs, name)
	return pathConf, err
}

func (s *Server) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (s *Server) doAuth(ctx *gin.Context, pathName string) bool {
	req := &auth.Request{
		Action:      conf.AuthActionPlayback,
		Path:        pathName,
		Query:       ctx.Request.URL.RawQuery,
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	_, err := s.AuthManager.Authenticate(req)
	if err != nil {
		if err.AskCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
			return false
		}

		s.Log(logger.Info, "connection %v failed to authenticate: %v",
			httpp.RemoteAddr(ctx), err.Wrapped)

		// wait some seconds to delay brute force attacks
		<-time.After(auth.PauseAfterError)

		s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
		return false
	}

	return true
}
