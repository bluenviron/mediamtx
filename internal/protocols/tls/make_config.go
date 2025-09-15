// Package tls contains TLS utilities.
package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
)

// MakeConfig returns a tls.Config with:
// - server name indicator (SNI) support
// - fingerprint support
func MakeConfig(serverName string, fingerprint string) *tls.Config {
	conf := &tls.Config{
		ServerName: serverName,
	}

	if fingerprint != "" {
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
	}

	return conf
}
