// Package tls contains TLS utilities.
package tls //nolint:revive

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
)

// MakeConfig returns a tls.Config with fingerprint support.
func MakeConfig(fingerprint string) *tls.Config {
	if fingerprint != "" {
		conf := &tls.Config{}

		fingerprintLower := strings.ToLower(fingerprint)
		conf.InsecureSkipVerify = true
		conf.VerifyConnection = func(cs tls.ConnectionState) error {
			h := sha256.New()
			h.Write(cs.PeerCertificates[0].Raw)
			hstr := hex.EncodeToString(h.Sum(nil))

			if hstr != fingerprintLower {
				return fmt.Errorf("source fingerprint does not match: expected %s, got %s",
					fingerprintLower, hstr)
			}

			return nil
		}

		return conf
	}

	return nil
}
