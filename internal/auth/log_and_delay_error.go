package auth

import (
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	minPause = 0 * time.Second
	maxPause = 4 * time.Second
)

// LogAndDelayError logs authentication errors and delays brute force attacks by waiting some seconds.
func LogAndDelayError(author logger.Writer, err error) {
	if terr, ok := errors.AsType[*Error](err); ok {
		if !terr.AskCredentials {
			author.Log(logger.Warn, err.Error())

			var n *big.Int
			n, err = rand.Int(rand.Reader, big.NewInt(int64(maxPause-minPause)))
			if err != nil {
				<-time.After(maxPause)
				return
			}
			<-time.After(minPause + time.Duration(n.Int64()))
		}
	}
}
