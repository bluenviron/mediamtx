package core

import (
	"github.com/gin-gonic/gin"
)

func httpServerHeaderMiddleware(ctx *gin.Context) {
	ctx.Writer.Header().Set("Server", "mediamtx")
	ctx.Next()
}
