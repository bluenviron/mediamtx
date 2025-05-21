package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

func mustParseCIDR(v string) net.IPNet {
	_, ne, err := net.ParseCIDR(v)
	if err != nil {
		panic(err)
	}
	if ipv4 := ne.IP.To4(); ipv4 != nil {
		return net.IPNet{IP: ipv4, Mask: ne.Mask[len(ne.Mask)-4 : len(ne.Mask)]}
	}
	return *ne
}

func strPointer(s string) *string {
	return &s
}

func TestMatchesPermission(t *testing.T) {
	testCases := []struct {
		name        string
		permissions []conf.AuthInternalUserPermission
		request     *Request
		expected    bool
	}{
		// 1. Old `path` format
		{
			name: "Old path - action match, path match exact",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("somepath")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "somepath"},
			expected: true,
		},
		{
			name: "Old path - action match, path match regex",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("~^somepath$")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "somepath"},
			expected: true,
		},
		{
			name: "Old path - action match, path match regex prefix",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("~^video")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "video/cam1"},
			expected: true,
		},
		{
			name: "Old path - action match, path match empty path for any",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "anotherpath"},
			expected: true,
		},
		{
			name: "Old path - action match, nil Path for any",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: nil},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "anotherpath"},
			expected: true,
		},
		{
			name: "Old path - action match, path no match",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("somepath")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "anotherpath"},
			expected: false,
		},
		{
			name: "Old path - action no match",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("somepath")},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "somepath"},
			expected: false,
		},

		// 2. New `paths` format
		{
			name: "New paths - action match, one path matches exact",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"path1", "path2", "path3"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "path2"},
			expected: true,
		},
		{
			name: "New paths - action match, one path matches regex",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"path1", "~^path2$", "path3"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "path2"},
			expected: true,
		},
		{
			name: "New paths - action match, one path matches regex prefix",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"other", "~^video"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "video/cam1"},
			expected: true,
		},
		{
			name: "New paths - action match, one path is empty string for any",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"path1", "", "path3"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "anyotherpath"},
			expected: true,
		},
		{
			name: "New paths - action match, multiple paths, one matches",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"nomatch1", "match", "nomatch2"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "match"},
			expected: true,
		},
		{
			name: "New paths - action match, Paths is empty slice (implies any path as Path is nil)",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Paths: []string{}}, // Path is implicitly nil
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "somepath"},
			expected: true,
		},
		{
			name: "New paths - action match, Paths is nil (implies any path as Path is nil)",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Paths: nil}, // Path is implicitly nil
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "somepath"},
			expected: true,
		},
		{
			name: "New paths - action match, path no match in Paths",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Paths: []string{"path1", "path2"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "path3"},
			expected: false,
		},
		{
			name: "New paths - action match, Paths with empty string, but Path has value (Paths takes precedence)",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Path: strPointer("specificPath"), Paths: []string{"", "path2"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "anyPathWillDo"},
			expected: true,
		},
		{
			name: "New paths - action match, Paths with no match, Path has value (Paths takes precedence, so no match)",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionRead, Path: strPointer("thisShouldNotMatch"), Paths: []string{"path1", "path2"}},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "thisShouldNotMatch"}, // Request path matches Path field, but Paths field takes precedence
			expected: false,
		},

		// 3. Permissions for non-path specific actions
		{
			name: "Non-path action - API, action match, path irrelevant",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionAPI},
			},
			request:  &Request{Action: conf.AuthActionAPI, Path: "anypath"},
			expected: true,
		},
		{
			name: "Non-path action - Metrics, action match, path irrelevant",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionMetrics},
			},
			request:  &Request{Action: conf.AuthActionMetrics, Path: "anypath/can/be/here"},
			expected: true,
		},
		{
			name: "Non-path action - Pprof, action match, path irrelevant",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPprof},
			},
			request:  &Request{Action: conf.AuthActionPprof, Path: ""},
			expected: true,
		},
		{
			name: "Non-path action - API, action no match",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionAPI},
			},
			request:  &Request{Action: conf.AuthActionMetrics, Path: "anypath"},
			expected: false,
		},
		{
			name: "Non-path action - API, permission has Path and Paths, should still match",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionAPI, Path: strPointer("somepath"), Paths: []string{"p1", "p2"}},
			},
			request:  &Request{Action: conf.AuthActionAPI, Path: "anypath"},
			expected: true,
		},

		// 4. Mixed permissions
		{
			name: "Mixed - First perm no match (action), second perm matches (path)",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("path1")},
				{Action: conf.AuthActionRead, Path: strPointer("path2")},
			},
			request:  &Request{Action: conf.AuthActionRead, Path: "path2"},
			expected: true,
		},
		{
			name: "Mixed - First perm matches (action and path), should return true without checking second",
			permissions: []conf.AuthInternalUserPermission{
				{Action: conf.AuthActionPublish, Path: strPointer("path1")},
				{Action: conf.AuthActionRead, Path: strPointer("path2")},
			},
			request:  &Request{Action: conf.AuthActionPublish, Path: "path1"},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matchesPermission(tc.permissions, tc.request)
			require.Equal(t, tc.expected, result)
		})
	}
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
								Path:   strPointer("mypath"), // Updated to use strPointer
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

				switch outcome {
				case "ok":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					})
					require.NoError(t, err)

				case "wrong user":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "wrong",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					})
					require.Error(t, err)

				case "wrong pass":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "wrong",
						},
						IP: net.ParseIP("127.1.1.1"),
					})
					require.Error(t, err)

				case "wrong ip":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionPublish,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.2"),
					})
					require.Error(t, err)

				case "wrong action":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionRead,
						Path:   "mypath",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					})
					require.Error(t, err)

				case "wrong path":
					err := m.Authenticate(&Request{
						Action: conf.AuthActionPublish,
						Path:   "wrong",
						Credentials: &Credentials{
							User: "testuser",
							Pass: "testpass",
						},
						IP: net.ParseIP("127.1.1.1"),
					})
					require.Error(t, err)
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
							Path:   strPointer("mypath"), // Updated to use strPointer
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
				require.NoError(t, err)
			} else {
				require.Error(t, err)
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

			if outcome == "ok" {
				err := m.Authenticate(&Request{
					Action:   conf.AuthActionPublish,
					Path:     "teststream",
					Query:    "param=value",
					Protocol: ProtocolRTSP,
					Credentials: &Credentials{
						User: "testpublisher",
						Pass: "testpass",
					},
					IP: net.ParseIP("127.0.0.1"),
				})
				require.NoError(t, err)
			} else {
				err := m.Authenticate(&Request{
					Action:   conf.AuthActionPublish,
					Path:     "teststream",
					Query:    "param=value",
					Protocol: ProtocolRTSP,
					Credentials: &Credentials{
						User: "invalid",
						Pass: "testpass",
					},
					IP: net.ParseIP("127.0.0.1"),
				})
				require.Error(t, err)
			}
		})
	}
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
	require.NoError(t, err)
}

func TestAuthJWT(t *testing.T) {
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
			Path:   strPointer("mypath"), // Updated to use strPointer
		}},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header[jwkset.HeaderKID] = "test-key-id"
	ss, err := token.SignedString(key)
	require.NoError(t, err)

	m := Manager{
		Method:      conf.AuthMethodJWT,
		JWTJWKS:     "http://localhost:4567/jwks",
		JWTClaimKey: "my_permission_key",
	}

	req := &Request{
		Action:   conf.AuthActionPublish,
		Path:     "mypath",
		Protocol: ProtocolWebRTC,
		Credentials: &Credentials{
			Token: ss,
		},
		IP: net.ParseIP("127.0.0.1"),
	}
	err = m.Authenticate(req)
	require.NoError(t, err)
}

func TestAuthJWTAsString(t *testing.T) {
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

	type customClaims struct {
		jwt.RegisteredClaims
		MediaMTXPermissions string `json:"my_permission_key"`
	}

	enc, err := json.Marshal([]conf.AuthInternalUserPermission{{
		Action: conf.AuthActionPublish,
		Path:   strPointer("mypath"), // Updated to use strPointer
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
	ss, err := token.SignedString(key)
	require.NoError(t, err)

	m := Manager{
		Method:      conf.AuthMethodJWT,
		JWTJWKS:     "http://localhost:4567/jwks",
		JWTClaimKey: "my_permission_key",
	}

	err = m.Authenticate(&Request{
		Action:   conf.AuthActionPublish,
		Path:     "mypath",
		Query:    "param=value",
		Protocol: ProtocolRTSP,
		Credentials: &Credentials{
			Token: ss,
		},
		IP: net.ParseIP("127.0.0.1"),
	})
	require.NoError(t, err)
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
	require.NoError(t, err)
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

	for i := 0; i < 2; i++ {
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
				Path:   strPointer("mypath"), // Updated to use strPointer
			}},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header[jwkset.HeaderKID] = "test-key-id"
		ss, err := token.SignedString(key)
		require.NoError(t, err)

		err = m.Authenticate(&Request{
			Action:   conf.AuthActionPublish,
			Path:     "mypath",
			Query:    "param=value",
			Protocol: ProtocolRTSP,
			Credentials: &Credentials{
				Token: ss,
			},
			IP: net.ParseIP("127.0.0.1"),
		})
		require.NoError(t, err)

		m.RefreshJWTJWKS()
	}
}
