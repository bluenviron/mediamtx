// Package auth contains the authentication system.
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// PauseAfterError is the pause to apply after an authentication failure.
	PauseAfterError = 2 * time.Second

	jwksRefreshPeriod = 60 * 60 * time.Second
)

func isHTTPRequest(r *Request) bool {
	switch r.Action {
	case conf.AuthActionPlayback, conf.AuthActionAPI,
		conf.AuthActionMetrics, conf.AuthActionPprof:
		return true
	}

	switch r.Protocol {
	case ProtocolHLS, ProtocolWebRTC:
		return true
	}

	return false
}

// Error is a authentication error.
type Error struct {
	Wrapped        error
	Message        string
	AskCredentials bool
}

// Error implements the error interface.
func (e Error) Error() string {
	return "authentication failed: " + e.Wrapped.Error()
}

func matchesPermission(perms []conf.AuthInternalUserPermission, req *Request) bool {
	for _, perm := range perms {
		if perm.Action == req.Action {
			if perm.Action == conf.AuthActionPublish ||
				perm.Action == conf.AuthActionRead ||
				perm.Action == conf.AuthActionPlayback {
				switch {
				case perm.Path == "":
					return true

				case strings.HasPrefix(perm.Path, "~"):
					regexp, err := regexp.Compile(perm.Path[1:])
					if err == nil && regexp.MatchString(req.Path) {
						return true
					}

				case perm.Path == req.Path:
					return true
				}
			} else {
				return true
			}
		}
	}

	return false
}

// Manager is the authentication manager.
type Manager struct {
	Method             conf.AuthMethod
	InternalUsers      []conf.AuthInternalUser
	HTTPAddress        string
	HTTPExclude        []conf.AuthInternalUserPermission
	JWTJWKS            string
	JWTJWKSFingerprint string
	JWTClaimKey        string
	JWTExclude         []conf.AuthInternalUserPermission
	JWTInHTTPQuery     bool
	ReadTimeout        time.Duration

	mutex           sync.RWMutex
	jwksLastRefresh time.Time
	jwtKeyFunc      keyfunc.Keyfunc
}

// ReloadInternalUsers reloads InternalUsers.
func (m *Manager) ReloadInternalUsers(u []conf.AuthInternalUser) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.InternalUsers = u
}

// Authenticate authenticates a request.
func (m *Manager) Authenticate(req *Request) error {
	var err error

	switch m.Method {
	case conf.AuthMethodInternal:
		err = m.authenticateInternal(req)

	case conf.AuthMethodHTTP:
		err = m.authenticateHTTP(req)

	default:
		err = m.authenticateJWT(req)
	}

	if err != nil {
		return Error{
			Wrapped:        err,
			AskCredentials: m.Method != conf.AuthMethodJWT && req.Credentials.User == "" && req.Credentials.Pass == "",
		}
	}

	return nil
}

func (m *Manager) authenticateInternal(req *Request) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, u := range m.InternalUsers {
		if ok := m.authenticateWithUser(req, &u); ok {
			return nil
		}
	}

	return fmt.Errorf("authentication failed")
}

func (m *Manager) authenticateWithUser(
	req *Request,
	u *conf.AuthInternalUser,
) bool {
	if len(u.IPs) != 0 && !u.IPs.Contains(req.IP) {
		return false
	}

	if !matchesPermission(u.Permissions, req) {
		return false
	}

	if u.User != "any" {
		if req.CustomVerifyFunc != nil {
			if ok := req.CustomVerifyFunc(string(u.User), string(u.Pass)); !ok {
				return false
			}
		} else {
			if !u.User.Check(req.Credentials.User) || !u.Pass.Check(req.Credentials.Pass) {
				return false
			}
		}
	}

	return true
}

func (m *Manager) authenticateHTTP(req *Request) error {
	if matchesPermission(m.HTTPExclude, req) {
		return nil
	}

	enc, _ := json.Marshal(struct {
		IP       string     `json:"ip"`
		User     string     `json:"user"`
		Password string     `json:"password"`
		Token    string     `json:"token"`
		Action   string     `json:"action"`
		Path     string     `json:"path"`
		Protocol string     `json:"protocol"`
		ID       *uuid.UUID `json:"id"`
		Query    string     `json:"query"`
	}{
		IP:       req.IP.String(),
		User:     req.Credentials.User,
		Password: req.Credentials.Pass,
		Token:    req.Credentials.Token,
		Action:   string(req.Action),
		Path:     req.Path,
		Protocol: string(req.Protocol),
		ID:       req.ID,
		Query:    req.Query,
	})

	res, err := http.Post(m.HTTPAddress, "application/json", bytes.NewReader(enc))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		if resBody, err := io.ReadAll(res.Body); err == nil && len(resBody) != 0 {
			return fmt.Errorf("server replied with code %d: %s", res.StatusCode, string(resBody))
		}

		return fmt.Errorf("server replied with code %d", res.StatusCode)
	}

	return nil
}

func (m *Manager) authenticateJWT(req *Request) error {
	if matchesPermission(m.JWTExclude, req) {
		return nil
	}

	keyfunc, err := m.pullJWTJWKS()
	if err != nil {
		return err
	}

	var encodedJWT string

	switch {
	case req.Credentials.Token != "":
		encodedJWT = req.Credentials.Token

	case req.Credentials.Pass != "":
		encodedJWT = req.Credentials.Pass

	case (!isHTTPRequest(req) || m.JWTInHTTPQuery):
		var v url.Values
		v, err = url.ParseQuery(req.Query)
		if err != nil {
			return err
		}

		if len(v["jwt"]) != 1 || len(v["jwt"][0]) == 0 {
			return fmt.Errorf("JWT not provided")
		}

		encodedJWT = v["jwt"][0]

	default:
		return fmt.Errorf("JWT not provided")
	}

	var cc jwtClaims
	cc.permissionsKey = m.JWTClaimKey
	_, err = jwt.ParseWithClaims(encodedJWT, &cc, keyfunc)
	if err != nil {
		return err
	}

	if !matchesPermission(cc.permissions, req) {
		return fmt.Errorf("user doesn't have permission to perform action")
	}

	return nil
}

func (m *Manager) pullJWTJWKS() (jwt.Keyfunc, error) {
	now := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if now.Sub(m.jwksLastRefresh) >= jwksRefreshPeriod {
		tr := &http.Transport{
			TLSClientConfig: tls.ConfigForFingerprint(m.JWTJWKSFingerprint),
		}
		defer tr.CloseIdleConnections()

		httpClient := &http.Client{
			Timeout:   (m.ReadTimeout),
			Transport: tr,
		}

		res, err := httpClient.Get(m.JWTJWKS)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		var raw json.RawMessage
		err = json.NewDecoder(res.Body).Decode(&raw)
		if err != nil {
			return nil, err
		}

		tmp, err := keyfunc.NewJWKSetJSON(raw)
		if err != nil {
			return nil, err
		}

		m.jwtKeyFunc = tmp
		m.jwksLastRefresh = now
	}

	return m.jwtKeyFunc.Keyfunc, nil
}

// RefreshJWTJWKS refreshes the JWT JWKS.
func (m *Manager) RefreshJWTJWKS() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.jwksLastRefresh = time.Time{}
}
