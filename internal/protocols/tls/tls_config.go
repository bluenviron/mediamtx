// Package tls contains TLS utilities.
package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
)

// ConfigForFingerprint returns a tls.Config that supports given fingerprint.
func ConfigForFingerprint(fingerprint string) *tls.Config {
	if fingerprint == "" {
		return nil
	}

	fingerprintLower := strings.ToLower(fingerprint)

	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			h := sha256.New()
			h.Write(cs.PeerCertificates[0].Raw)
			hstr := hex.EncodeToString(h.Sum(nil))

			if hstr != fingerprintLower {
				return fmt.Errorf("source fingerprint does not match: expected %s, got %s",
					fingerprintLower, hstr)
			}

			return nil
		},
	}
}
