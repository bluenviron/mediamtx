// Package playback contains the playback server.
package playback

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/gin-gonic/gin"
)

var errNoSegmentsFound = errors.New("no recording segments found for the given timestamp")

// Server is the playback server.
type Server struct {
	Address     string
	ReadTimeout conf.StringDuration
	PathConfs   map[string]*conf.Path
	AuthManager *auth.Manager
	Parent      logger.Writer

	httpServer *httpp.WrappedServer
	mutex      sync.RWMutex
}

// Initialize initializes Server.
func (p *Server) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(nil) //nolint:errcheck

	group := router.Group("/")

	group.GET("/list", p.onList)
	group.GET("/get", p.onGet)

	network, address := restrictnetwork.Restrict("tcp", p.Address)

	var err error
	p.httpServer, err = httpp.NewWrappedServer(
		network,
		address,
		time.Duration(p.ReadTimeout),
		"",
		"",
		router,
		p,
	)
	if err != nil {
		return err
	}

	p.Log(logger.Info, "listener opened on "+address)

	return nil
}

// Close closes Server.
func (p *Server) Close() {
	p.Log(logger.Info, "listener is closing")
	p.httpServer.Close()
}

// Log implements logger.Writer.
func (p *Server) Log(level logger.Level, format string, args ...interface{}) {
	p.Parent.Log(level, "[playback] "+format, args...)
}

// ReloadPathConfs is called by core.Core.
func (p *Server) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.PathConfs = pathConfs
}

func (p *Server) writeError(ctx *gin.Context, status int, err error) {
	// show error in logs
	p.Log(logger.Error, err.Error())

	// add error to response
	ctx.String(status, err.Error())
}

func (p *Server) safeFindPathConf(name string) (*conf.Path, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	_, pathConf, _, err := conf.FindPathConf(p.PathConfs, name)
	return pathConf, err
}

func (p *Server) doAuth(ctx *gin.Context, pathName string) bool {
	user, pass, hasCredentials := ctx.Request.BasicAuth()

	err := p.AuthManager.Authenticate(&auth.Request{
		User:   user,
		Pass:   pass,
		IP:     net.ParseIP(ctx.ClientIP()),
		Action: conf.AuthActionPlayback,
		Path:   pathName,
	})
	if err != nil {
		if !hasCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.Writer.WriteHeader(http.StatusUnauthorized)
			return false
		}

		var terr auth.Error
		errors.As(err, &terr)

		p.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Message)

		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.Writer.WriteHeader(http.StatusUnauthorized)
		return false
	}

	return true
}
