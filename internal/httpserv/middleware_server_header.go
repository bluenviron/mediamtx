package httpserv

import (
	"github.com/gin-gonic/gin"
)

// MiddlewareServerHeader is a middleware that sets the Server header.
func MiddlewareServerHeader(ctx *gin.Context) {
	ctx.Writer.Header().Set("Server", "mediamtx")
	ctx.Next()
}
