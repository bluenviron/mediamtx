package conf

// TLSCert is a TLS key pair.
type TLSCert struct {
	Key  string `json:"key"`
	Cert string `json:"cert"`
}
