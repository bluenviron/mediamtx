package auth

import (
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

const (
	minPause = 0 * time.Second
	maxPause = 4 * time.Second
)

// DelayBruteForce delays brute force attacks by waiting some seconds after an authentication error.
func DelayBruteForce(err error) {
	if terr, ok := errors.AsType[*Error](err); ok {
		if !terr.AskCredentials {
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
