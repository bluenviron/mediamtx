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

	maxInboundBodySize = 128 * 1024
	jwksRefreshPeriod  = 60 * 60 * time.Second
)

func isHTTP(req *Request) bool {
	return req.Protocol == ProtocolHLS || req.Protocol == ProtocolWebRTC ||
		req.Action == conf.AuthActionPlayback ||
		req.Action == conf.AuthActionAPI ||
		req.Action == conf.AuthActionMetrics ||
		req.Action == conf.AuthActionPprof
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

func getToken(tokenInHTTPQuery bool, req *Request) string {
	switch {
	case req.Credentials.Token != "":
		return req.Credentials.Token

	case req.Credentials.Pass != "":
		return req.Credentials.Pass

		// always allow passing tokens through query parameters with RTSP and RTMP since there's no alternative.
	case req.Protocol == ProtocolRTSP || req.Protocol == ProtocolRTMP ||
		(tokenInHTTPQuery && isHTTP(req)):
		v, err := url.ParseQuery(req.Query)
		if err == nil {
			if len(v["token"]) == 1 {
				return v["token"][0]
			}

			// legacy query key
			if len(v["jwt"]) == 1 {
				return v["jwt"][0]
			}
		}
	}

	return ""
}

// Manager is the authentication manager.
type Manager struct {
	Method             conf.AuthMethod
	InternalUsers      []conf.AuthInternalUser
	HTTPAddress        string
	HTTPFingerprint    string
	HTTPExclude        []conf.AuthInternalUserPermission
	JWTJWKS            string
	JWTJWKSFingerprint string
	JWTClaimKey        string
	JWTExclude         []conf.AuthInternalUserPermission
	JWTInHTTPQuery     *bool
	JWTIssuer          string
	JWTAudience        string
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
// It returns the user name.
func (m *Manager) Authenticate(req *Request) (string, *Error) {
	var token string
	if m.Method == conf.AuthMethodHTTP || m.Method == conf.AuthMethodJWT {
		token = getToken(m.Method == conf.AuthMethodJWT && m.JWTInHTTPQuery != nil && *m.JWTInHTTPQuery, req)
	}

	var user string
	var err error

	switch m.Method {
	case conf.AuthMethodInternal:
		user, err = m.authenticateInternal(req)

	case conf.AuthMethodHTTP:
		user, err = m.authenticateHTTP(req, token)

	default:
		user, err = m.authenticateJWT(req, token)
	}

	if err != nil {
		return "", &Error{
			Wrapped:        err,
			AskCredentials: (req.Credentials.User == "" && req.Credentials.Pass == "" && token == ""),
		}
	}

	return user, nil
}

func (m *Manager) authenticateInternal(req *Request) (string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, u := range m.InternalUsers {
		if ok := m.authenticateWithUser(req, &u); ok {
			return req.Credentials.User, nil
		}
	}

	return "", fmt.Errorf("authentication failed")
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

func (m *Manager) authenticateHTTP(req *Request, token string) (string, error) {
	if matchesPermission(m.HTTPExclude, req) {
		return "", nil
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
		Token:    token,
		Action:   string(req.Action),
		Path:     req.Path,
		Protocol: string(req.Protocol),
		ID:       req.ID,
		Query:    req.Query,
	})

	tr := &http.Transport{
		TLSClientConfig: tls.MakeConfig(m.HTTPFingerprint),
	}
	defer tr.CloseIdleConnections()

	httpClient := &http.Client{
		Timeout:   m.ReadTimeout,
		Transport: tr,
	}

	res, err := httpClient.Post(m.HTTPAddress, "application/json", bytes.NewReader(enc))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		resBody, err2 := io.ReadAll(&customLimitReader{res.Body, maxInboundBodySize})
		if err2 == nil && len(resBody) != 0 {
			return "", fmt.Errorf("server replied with code %d: %s", res.StatusCode, string(resBody))
		}

		return "", fmt.Errorf("server replied with code %d", res.StatusCode)
	}

	return req.Credentials.User, nil
}

func (m *Manager) authenticateJWT(req *Request, token string) (string, error) {
	if matchesPermission(m.JWTExclude, req) {
		return "", nil
	}

	keyfunc, err := m.pullJWTJWKS()
	if err != nil {
		return "", err
	}

	if token == "" {
		return "", fmt.Errorf("JWT not provided")
	}

	var opts []jwt.ParserOption
	if m.JWTIssuer != "" {
		opts = append(opts, jwt.WithIssuer(m.JWTIssuer))
	}
	if m.JWTAudience != "" {
		opts = append(opts, jwt.WithAudience(m.JWTAudience))
	}

	var cc jwtClaims
	cc.permissionsKey = m.JWTClaimKey
	_, err = jwt.ParseWithClaims(token, &cc, keyfunc, opts...)
	if err != nil {
		return "", err
	}

	if !matchesPermission(cc.permissions, req) {
		return "", fmt.Errorf("user doesn't have permission to perform action")
	}

	return cc.Subject, nil
}

func (m *Manager) pullJWTJWKS() (jwt.Keyfunc, error) {
	now := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if now.Sub(m.jwksLastRefresh) >= jwksRefreshPeriod {
		tr := &http.Transport{
			TLSClientConfig: tls.MakeConfig(m.JWTJWKSFingerprint),
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
		err = json.NewDecoder(&customLimitReader{res.Body, maxInboundBodySize}).Decode(&raw)
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
