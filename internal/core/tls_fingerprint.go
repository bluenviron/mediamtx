package core

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
)

type fingerprintValidatorFunc func(tls.ConnectionState) error

func fingerprintValidator(fingerprint string) fingerprintValidatorFunc {
	fingerprintLower := strings.ToLower(fingerprint)

	return func(cs tls.ConnectionState) error {
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

func tlsConfigForFingerprint(fingerprint string) *tls.Config {
	if fingerprint == "" {
		return nil
	}

	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection:   fingerprintValidator(fingerprint),
	}
}
