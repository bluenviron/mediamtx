package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

var testTLSCertPub = []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXw1hEC3LFpTsllv7D3ARJyEq7sIwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMDEyMTMxNzQ0NThaFw0zMDEy
MTExNzQ0NThaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDG8DyyS51810GsGwgWr5rjJK7OE1kTTLSNEEKax8Bj
zOyiaz8rA2JGl2VUEpi2UjDr9Cm7nd+YIEVs91IIBOb7LGqObBh1kGF3u5aZxLkv
NJE+HrLVvUhaDobK2NU+Wibqc/EI3DfUkt1rSINvv9flwTFu1qHeuLWhoySzDKEp
OzYxpFhwjVSokZIjT4Red3OtFz7gl2E6OAWe2qoh5CwLYVdMWtKR0Xuw3BkDPk9I
qkQKx3fqv97LPEzhyZYjDT5WvGrgZ1WDAN3booxXF3oA1H3GHQc4m/vcLatOtb8e
nI59gMQLEbnp08cl873bAuNuM95EZieXTHNbwUnq5iybAgMBAAGjUzBRMB0GA1Ud
DgQWBBQBKhJh8eWu0a4au9X/2fKhkFX2vjAfBgNVHSMEGDAWgBQBKhJh8eWu0a4a
u9X/2fKhkFX2vjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBj
3aCW0YPKukYgVK9cwN0IbVy/D0C1UPT4nupJcy/E0iC7MXPZ9D/SZxYQoAkdptdO
xfI+RXkpQZLdODNx9uvV+cHyZHZyjtE5ENu/i5Rer2cWI/mSLZm5lUQyx+0KZ2Yu
tEI1bsebDK30msa8QSTn0WidW9XhFnl3gRi4wRdimcQapOWYVs7ih+nAlSvng7NI
XpAyRs8PIEbpDDBMWnldrX4TP6EWYUi49gCp8OUDRREKX3l6Ls1vZ02F34yHIt/7
7IV/XSKG096bhW+icKBWV0IpcEsgTzPK1J1hMxgjhzIMxGboAeUU+kidthOob6Sd
XQxaORfgM//NzX9LhUPk
-----END CERTIFICATE-----
`)

var testTLSCertKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAxvA8skudfNdBrBsIFq+a4ySuzhNZE0y0jRBCmsfAY8zsoms/
KwNiRpdlVBKYtlIw6/Qpu53fmCBFbPdSCATm+yxqjmwYdZBhd7uWmcS5LzSRPh6y
1b1IWg6GytjVPlom6nPxCNw31JLda0iDb7/X5cExbtah3ri1oaMkswyhKTs2MaRY
cI1UqJGSI0+EXndzrRc+4JdhOjgFntqqIeQsC2FXTFrSkdF7sNwZAz5PSKpECsd3
6r/eyzxM4cmWIw0+Vrxq4GdVgwDd26KMVxd6ANR9xh0HOJv73C2rTrW/HpyOfYDE
CxG56dPHJfO92wLjbjPeRGYnl0xzW8FJ6uYsmwIDAQABAoIBACi0BKcyQ3HElSJC
kaAao+Uvnzh4yvPg8Nwf5JDIp/uDdTMyIEWLtrLczRWrjGVZYbsVROinP5VfnPTT
kYwkfKINj2u+gC6lsNuPnRuvHXikF8eO/mYvCTur1zZvsQnF5kp4GGwIqr+qoPUP
bB0UMndG1PdpoMryHe+JcrvTrLHDmCeH10TqOwMsQMLHYLkowvxwJWsmTY7/Qr5S
Wm3PPpOcW2i0uyPVuyuv4yD1368fqnqJ8QFsQp1K6QtYsNnJ71Hut1/IoxK/e6hj
5Z+byKtHVtmcLnABuoOT7BhleJNFBksX9sh83jid4tMBgci+zXNeGmgqo2EmaWAb
agQslkECgYEA8B1rzjOHVQx/vwSzDa4XOrpoHQRfyElrGNz9JVBvnoC7AorezBXQ
M9WTHQIFTGMjzD8pb+YJGi3gj93VN51r0SmJRxBaBRh1ZZI9kFiFzngYev8POgD3
ygmlS3kTHCNxCK/CJkB+/jMBgtPj5ygDpCWVcTSuWlQFphePkW7jaaECgYEA1Blz
ulqgAyJHZaqgcbcCsI2q6m527hVr9pjzNjIVmkwu38yS9RTCgdlbEVVDnS0hoifl
+jVMEGXjF3xjyMvL50BKbQUH+KAa+V4n1WGlnZOxX9TMny8MBjEuSX2+362vQ3BX
4vOlX00gvoc+sY+lrzvfx/OdPCHQGVYzoKCxhLsCgYA07HcviuIAV/HsO2/vyvhp
xF5gTu+BqNUHNOZDDDid+ge+Jre2yfQLCL8VPLXIQW3Jff53IH/PGl+NtjphuLvj
7UDJvgvpZZuymIojP6+2c3gJ3CASC9aR3JBnUzdoE1O9s2eaoMqc4scpe+SWtZYf
3vzSZ+cqF6zrD/Rf/M35IQKBgHTU4E6ShPm09CcoaeC5sp2WK8OevZw/6IyZi78a
r5Oiy18zzO97U/k6xVMy6F+38ILl/2Rn31JZDVJujniY6eSkIVsUHmPxrWoXV1HO
y++U32uuSFiXDcSLarfIsE992MEJLSAynbF1Rsgsr3gXbGiuToJRyxbIeVy7gwzD
94TpAoGAY4/PejWQj9psZfAhyk5dRGra++gYRQ/gK1IIc1g+Dd2/BxbT/RHr05GK
6vwrfjsoRyMWteC1SsNs/CurjfQ/jqCfHNP5XPvxgd5Ec8sRJIiV7V5RTuWJsPu1
+3K6cnKEyg+0ekYmLertRFIY6SwWmY1fyKgTvxudMcsBY7dC4xs=
-----END RSA PRIVATE KEY-----
`)

func mustParseCIDR(v string) conf.IPNetwork {
	_, ne, err := net.ParseCIDR(v)
	if err != nil {
		panic(err)
	}
	if ipv4 := ne.IP.To4(); ipv4 != nil {
		return conf.IPNetwork{IP: ipv4, Mask: ne.Mask[len(ne.Mask)-4 : len(ne.Mask)]}
	}
	return conf.IPNetwork(*ne)
}

func TestAuthInternal(t *testing.T) {
	for _, outcome := range []string{
		"ok",
		"wrong user",
		"wrong pass",
		"wrong ip",
		"wrong action",
		"wrong path",
	} {
		for _, encryption := range []string{
			"plain",
			"sha256",
			"argon2",
		} {
			t.Run(outcome+" "+encryption, func(t *testing.T) {
				m := Manager{
					Method: conf.AuthMethodInternal,
					InternalUsers: []conf.AuthInternalUser{
						{
							IPs: conf.IPNetworks{mustParseCIDR("127.1.1.1/32")},
							Permissions: []conf.AuthInternalUserPermission{{
								Action: conf.AuthActionPublish,
								Path:   "mypath",
							}},
						},
					},
				}

				switch encryption {
				case "plain":
					m.InternalUsers[0].User = conf.Credential("testuser")
					m.InternalUsers[0].Pass = conf.Credential("testpass")

				case "sha256":
					m.InternalUsers[0].User = conf.Credential("sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=")
					m.InternalUsers[0].Pass = conf.Credential("sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=")

				case "argon2":
					m.InternalUsers[0].User = conf.Credential(
						"argon2:$argon2id$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$Ux/LWeTgJQPyfMMJo1myR64+o8rALHoPmlE1i/TR+58")
					m.InternalUsers[0].Pass = conf.Credential(
						"argon2:$argon2i$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$/mrZ42TiTv1mcPnpMUera5oi0SFYbbyueAbdx5sUvWo")
				}

				var req *Request

				switch outcome {
				case "ok":
					req = &Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					}

				case "wrong user":
					req = &Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "wrong",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					}

				case "wrong pass":
					req = &Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "wrong",
						},
						IP: net.ParseIP("127.1.1.1"),
					}

				case "wrong ip":
					req = &Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.2"),
					}

				case "wrong action":
					req = &Request{
						Action: conf.AuthActionRead,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					}

				case "wrong path":
					req = &Request{
						Action: conf.AuthActionPublish,
						Path:   "wrong",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					}
				}

				// first request with empty credentials
				err := m.Authenticate(&Request{
					Action:      req.Action,
					Path:        req.Path,
					Credentials: &Credentials{},
					IP:          req.IP,
				})
				require.Equal(t, &Error{
					Wrapped:        err.Wrapped,
					AskCredentials: true,
				}, err)

				// second request
				err = m.Authenticate(req)
				if outcome == "ok" {
					require.Nil(t, err)
				} else {
					require.EqualError(t, err.Wrapped, "authentication failed")
					require.False(t, err.AskCredentials)
				}
			})
		}
	}
}

func TestAuthInternalCustomVerifyFunc(t *testing.T) {
	for _, ca := range []string{"ok", "invalid"} {
		t.Run(ca, func(t *testing.T) {
			m := Manager{
				Method: conf.AuthMethodInternal,
				InternalUsers: []conf.AuthInternalUser{
					{
						User: "myuser",
						Pass: "mypass",
						IPs:  conf.IPNetworks{mustParseCIDR("127.1.1.1/32")},
						Permissions: []conf.AuthInternalUserPermission{{
							Action: conf.AuthActionPublish,
							Path:   "mypath",
						}},
					},
				},
			}

			req1 := &Request{
				Action:      conf.AuthActionPublish,
				Path:        "mypath",
				Credentials: &Credentials{},
				IP:          net.ParseIP("127.1.1.1"),
				CustomVerifyFunc: func(expectedUser, expectedPass string) bool {
					require.Equal(t, "myuser", expectedUser)
					require.Equal(t, "mypass", expectedPass)
					return (ca == "ok")
				},
			}
			err := m.Authenticate(req1)
			if ca == "ok" {
				require.Nil(t, err)
			} else {
				require.EqualError(t, err.Wrapped, "authentication failed")
			}
		})
	}
}

func TestAuthHTTP(t *testing.T) {
	for _, outcome := range []string{"ok", "fail"} {
		t.Run(outcome, func(t *testing.T) {
			firstReceived := false

			httpServ := &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "/auth", r.URL.Path)

					var in struct {
						IP       string `json:"ip"`
						User     string `json:"user"`
						Password string `json:"password"`
						Path     string `json:"path"`
						Protocol string `json:"protocol"`
						ID       string `json:"id"`
						Action   string `json:"action"`
						Query    string `json:"query"`
					}
					err := json.NewDecoder(r.Body).Decode(&in)
					require.NoError(t, err)

					if in.IP != "127.0.0.1" ||
						in.User != "testpublisher" ||
						in.Password != "testpass" ||
						in.Path != "teststream" ||
						in.Protocol != "rtsp" ||
						(firstReceived && in.ID == "") ||
						in.Action != "publish" ||
						(in.Query != "user=testreader&pass=testpass&param=value" &&
							in.Query != "user=testpublisher&pass=testpass&param=value" &&
							in.Query != "param=value") {
						w.WriteHeader(http.StatusBadRequest)
						return
					}

					firstReceived = true
				}),
			}

			ln, err := net.Listen("tcp", "127.0.0.1:9120")
			require.NoError(t, err)

			go httpServ.Serve(ln)
			defer httpServ.Shutdown(context.Background())

			m := Manager{
				Method:      conf.AuthMethodHTTP,
				HTTPAddress: "http://127.0.0.1:9120/auth",
			}

			var req *Request

			if outcome == "ok" {
				req = &Request{
					Action:   conf.AuthActionPublish,
					Path:     "teststream",
					Query:    "param=value",
					Protocol: ProtocolRTSP,
					Credentials: &Credentials{
						User: "testpublisher",
						Pass: "testpass",
					},
					IP: net.ParseIP("127.0.0.1"),
				}
			} else {
				req = &Request{
					Action:   conf.AuthActionPublish,
					Path:     "teststream",
					Query:    "param=value",
					Protocol: ProtocolRTSP,
					Credentials: &Credentials{
						User: "invalid",
						Pass: "testpass",
					},
					IP: net.ParseIP("127.0.0.1"),
				}
			}

			// first request with empty credentials
			err2 := m.Authenticate(&Request{
				Action:      req.Action,
				Path:        req.Path,
				Credentials: &Credentials{},
				IP:          req.IP,
			})
			require.Equal(t, &Error{
				Wrapped:        err2.Wrapped,
				AskCredentials: true,
			}, err2)

			// second request
			err2 = m.Authenticate(req)
			if outcome == "ok" {
				require.Nil(t, err2)
			} else {
				require.EqualError(t, err2.Wrapped, "server replied with code 400")
				require.False(t, err2.AskCredentials)
			}
		})
	}
}

func TestAuthHTTPFingerprint(t *testing.T) {
	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/auth", r.URL.Path)

			var in struct {
				IP       string `json:"ip"`
				User     string `json:"user"`
				Password string `json:"password"`
				Path     string `json:"path"`
				Protocol string `json:"protocol"`
				ID       string `json:"id"`
				Action   string `json:"action"`
				Query    string `json:"query"`
			}
			err := json.NewDecoder(r.Body).Decode(&in)
			require.NoError(t, err)

			if in.User != "testuser" || in.Password != "testpass" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:9121")
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(testTLSCertPub, testTLSCertKey)
	require.NoError(t, err)

	httpServ.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}

	go httpServ.ServeTLS(ln, "", "")
	defer httpServ.Shutdown(context.Background())

	m := Manager{
		Method:          conf.AuthMethodHTTP,
		HTTPAddress:     "https://localhost:9121/auth",
		HTTPFingerprint: "33949e05fffb5ff3e8aa16f8213a6251b4d9363804ba53233c4da9a46d6f2739",
	}

	err2 := m.Authenticate(&Request{
		Action:   conf.AuthActionPublish,
		Path:     "teststream",
		Protocol: ProtocolRTSP,
		Credentials: &Credentials{
			User: "testuser",
			Pass: "testpass",
		},
		IP: net.ParseIP("127.0.0.1"),
	})
	require.Nil(t, err2)
}

func TestAuthHTTPExclude(t *testing.T) {
	m := Manager{
		Method:      conf.AuthMethodHTTP,
		HTTPAddress: "http://not-to-be-used:9120/auth",
		HTTPExclude: []conf.AuthInternalUserPermission{{
			Action: conf.AuthActionPublish,
		}},
	}

	err := m.Authenticate(&Request{
		Action:   conf.AuthActionPublish,
		Path:     "teststream",
		Query:    "param=value",
		Protocol: ProtocolRTSP,
		Credentials: &Credentials{
			User: "",
			Pass: "",
		},
		IP: net.ParseIP("127.0.0.1"),
	})
	require.Nil(t, err)
}

func TestAuthJWT(t *testing.T) {
	for _, ca := range []string{"object", "string"} {
		t.Run(ca, func(t *testing.T) {
			// reference:
			// https://github.com/MicahParks/jwkset/blob/master/examples/http_server/main.go

			key, err := rsa.GenerateKey(rand.Reader, 1024)
			require.NoError(t, err)

			httpServ := &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					jwk, err2 := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{
						Metadata: jwkset.JWKMetadataOptions{
							KID: "test-key-id",
						},
					})
					require.NoError(t, err2)

					jwkSet := jwkset.NewMemoryStorage()
					err2 = jwkSet.KeyWrite(context.Background(), jwk)
					require.NoError(t, err2)

					response, err2 := jwkSet.JSONPublic(r.Context())
					if err2 != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(response)
				}),
			}

			ln, err := net.Listen("tcp", "localhost:4567")
			require.NoError(t, err)

			go httpServ.Serve(ln)
			defer httpServ.Shutdown(context.Background())

			var req *Request

			if ca == "object" {
				type customClaims struct {
					jwt.RegisteredClaims
					MediaMTXPermissions string `json:"my_permission_key"`
				}

				var enc []byte
				enc, err = json.Marshal([]conf.AuthInternalUserPermission{{
					Action: conf.AuthActionPublish,
					Path:   "mypath",
				}})
				require.NoError(t, err)

				claims := customClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
						Issuer:    "test",
						Subject:   "somebody",
						ID:        "1",
					},
					MediaMTXPermissions: string(enc),
				}

				token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
				token.Header[jwkset.HeaderKID] = "test-key-id"
				var ss string
				ss, err = token.SignedString(key)
				require.NoError(t, err)

				req = &Request{
					Action:   conf.AuthActionPublish,
					Path:     "mypath",
					Query:    "param=value",
					Protocol: ProtocolRTSP,
					Credentials: &Credentials{
						Token: ss,
					},
					IP: net.ParseIP("127.0.0.1"),
				}
			} else {
				type customClaims struct {
					jwt.RegisteredClaims
					MediaMTXPermissions []conf.AuthInternalUserPermission `json:"my_permission_key"`
				}

				claims := customClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
						Issuer:    "test",
						Subject:   "somebody",
						ID:        "1",
					},
					MediaMTXPermissions: []conf.AuthInternalUserPermission{{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
					}},
				}

				token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
				token.Header[jwkset.HeaderKID] = "test-key-id"
				var ss string
				ss, err = token.SignedString(key)
				require.NoError(t, err)

				req = &Request{
					Action:   conf.AuthActionPublish,
					Path:     "mypath",
					Protocol: ProtocolWebRTC,
					Credentials: &Credentials{
						Token: ss,
					},
					IP: net.ParseIP("127.0.0.1"),
				}
			}

			m := Manager{
				Method:      conf.AuthMethodJWT,
				JWTJWKS:     "http://localhost:4567/jwks",
				JWTClaimKey: "my_permission_key",
			}

			// first request with empty credentials
			err2 := m.Authenticate(&Request{
				Action:      req.Action,
				Path:        req.Path,
				Credentials: &Credentials{},
				IP:          req.IP,
			})
			require.Equal(t, &Error{
				Wrapped:        err2.Wrapped,
				AskCredentials: true,
			}, err2)

			// second request
			err2 = m.Authenticate(req)
			require.Nil(t, err2)
		})
	}
}

func TestAuthJWTExclude(t *testing.T) {
	m := Manager{
		Method:      conf.AuthMethodJWT,
		JWTJWKS:     "http://localhost:4567/jwks",
		JWTClaimKey: "my_permission_key",
		JWTExclude: []conf.AuthInternalUserPermission{{
			Action: conf.AuthActionPublish,
		}},
	}

	err := m.Authenticate(&Request{
		Action:   conf.AuthActionPublish,
		Path:     "teststream",
		Query:    "param=value",
		Protocol: ProtocolRTSP,
		IP:       net.ParseIP("127.0.0.1"),
	})
	require.Nil(t, err)
}

func TestAuthJWTRefresh(t *testing.T) {
	// reference:
	// https://github.com/MicahParks/jwkset/blob/master/examples/http_server/main.go

	var key *rsa.PrivateKey

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwk, err := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{
				Metadata: jwkset.JWKMetadataOptions{
					KID: "test-key-id",
				},
			})
			require.NoError(t, err)

			jwkSet := jwkset.NewMemoryStorage()
			err = jwkSet.KeyWrite(context.Background(), jwk)
			require.NoError(t, err)

			response, err2 := jwkSet.JSONPublic(r.Context())
			if err2 != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(response)
		}),
	}

	ln, err := net.Listen("tcp", "localhost:4567")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	m := Manager{
		Method:      conf.AuthMethodJWT,
		JWTJWKS:     "http://localhost:4567/jwks",
		JWTClaimKey: "my_permission_key",
	}

	for range 2 {
		key, err = rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		type customClaims struct {
			jwt.RegisteredClaims
			MediaMTXPermissions []conf.AuthInternalUserPermission `json:"my_permission_key"`
		}

		claims := customClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
				Issuer:    "test",
				Subject:   "somebody",
				ID:        "1",
			},
			MediaMTXPermissions: []conf.AuthInternalUserPermission{{
				Action: conf.AuthActionPublish,
				Path:   "mypath",
			}},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header[jwkset.HeaderKID] = "test-key-id"
		var ss string
		ss, err = token.SignedString(key)
		require.NoError(t, err)

		err2 := m.Authenticate(&Request{
			Action:   conf.AuthActionPublish,
			Path:     "mypath",
			Query:    "param=value",
			Protocol: ProtocolRTSP,
			Credentials: &Credentials{
				Token: ss,
			},
			IP: net.ParseIP("127.0.0.1"),
		})
		require.Nil(t, err2)

		m.RefreshJWTJWKS()
	}
}

func TestAuthJWTFingerprint(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwk, err2 := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{
				Metadata: jwkset.JWKMetadataOptions{
					KID: "test-key-id",
				},
			})
			require.NoError(t, err2)

			jwkSet := jwkset.NewMemoryStorage()
			err2 = jwkSet.KeyWrite(context.Background(), jwk)
			require.NoError(t, err2)

			response, err2 := jwkSet.JSONPublic(r.Context())
			if err2 != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(response)
		}),
	}

	ln, err := net.Listen("tcp", "localhost:4568")
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(testTLSCertPub, testTLSCertKey)
	require.NoError(t, err)

	httpServ.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}

	go httpServ.ServeTLS(ln, "", "")
	defer httpServ.Shutdown(context.Background())

	type customClaims struct {
		jwt.RegisteredClaims
		MediaMTXPermissions []conf.AuthInternalUserPermission `json:"my_permission_key"`
	}

	claims := customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
		},
		MediaMTXPermissions: []conf.AuthInternalUserPermission{{
			Action: conf.AuthActionPublish,
			Path:   "mypath",
		}},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header[jwkset.HeaderKID] = "test-key-id"
	ss, err := token.SignedString(key)
	require.NoError(t, err)

	m := Manager{
		Method:             conf.AuthMethodJWT,
		JWTJWKS:            "https://localhost:4568/jwks",
		JWTJWKSFingerprint: "33949e05fffb5ff3e8aa16f8213a6251b4d9363804ba53233c4da9a46d6f2739",
		JWTClaimKey:        "my_permission_key",
	}

	err2 := m.Authenticate(&Request{
		Action:   conf.AuthActionPublish,
		Path:     "mypath",
		Protocol: ProtocolRTSP,
		Credentials: &Credentials{
			Token: ss,
		},
		IP: net.ParseIP("127.0.0.1"),
	})
	require.Nil(t, err2)
}
