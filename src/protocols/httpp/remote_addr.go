package httpp

import (
	"net"

	"github.com/gin-gonic/gin"
)

// RemoteAddr returns the remote address of an HTTP client,
// with the IP replaced by the real IP passed by any proxy in between.
func RemoteAddr(ctx *gin.Context) string {
	ip := ctx.ClientIP()
	_, port, _ := net.SplitHostPort(ctx.Request.RemoteAddr)
	return net.JoinHostPort(ip, port)
}
