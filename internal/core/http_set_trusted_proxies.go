package core

import (
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func httpSetTrustedProxies(router *gin.Engine, trustedProxies conf.IPsOrCIDRs) {
	tmp := make([]string, len(trustedProxies))
	for i, entry := range trustedProxies {
		tmp[i] = entry.String()
	}
	router.SetTrustedProxies(tmp)
}
