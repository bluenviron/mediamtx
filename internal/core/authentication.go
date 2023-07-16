package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/bluenviron/gortsplib/v3/pkg/auth"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/headers"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func sha256Base64(in string) string {
	h := sha256.New()
	h.Write([]byte(in))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func checkCredential(right string, guess string) bool {
	if strings.HasPrefix(right, "sha256:") {
		return right[len("sha256:"):] == sha256Base64(guess)
	}

	return right == guess
}

type errAuthentication struct {
	message string
}

// Error implements the error interface.
func (e *errAuthentication) Error() string {
	return "authentication failed: " + e.message
}

type authProtocol string

const (
	authProtocolRTSP   authProtocol = "rtsp"
	authProtocolRTMP   authProtocol = "rtmp"
	authProtocolHLS    authProtocol = "hls"
	authProtocolWebRTC authProtocol = "webrtc"
	authProtocolSRT    authProtocol = "srt"
)

type authCredentials struct {
	query       string
	ip          net.IP
	user        string
	pass        string
	proto       authProtocol
	id          *uuid.UUID
	rtspRequest *base.Request
	rtspBaseURL *url.URL
	rtspNonce   string
}

func doExternalAuthentication(
	ur string,
	path string,
	publish bool,
	credentials authCredentials,
) error {
	enc, _ := json.Marshal(struct {
		IP       string     `json:"ip"`
		User     string     `json:"user"`
		Password string     `json:"password"`
		Path     string     `json:"path"`
		Protocol string     `json:"protocol"`
		ID       *uuid.UUID `json:"id"`
		Action   string     `json:"action"`
		Query    string     `json:"query"`
	}{
		IP:       credentials.ip.String(),
		User:     credentials.user,
		Password: credentials.pass,
		Path:     path,
		Protocol: string(credentials.proto),
		ID:       credentials.id,
		Action: func() string {
			if publish {
				return "publish"
			}
			return "read"
		}(),
		Query: credentials.query,
	})
	res, err := http.Post(ur, "application/json", bytes.NewReader(enc))
	if err != nil {
		return err
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

func doAuthentication(
	externalAuthenticationURL string,
	rtspAuthMethods conf.AuthMethods,
	pathName string,
	pathConf *conf.PathConf,
	publish bool,
	credentials authCredentials,
) error {
	var rtspAuth headers.Authorization
	if credentials.rtspRequest != nil {
		err := rtspAuth.Unmarshal(credentials.rtspRequest.Header["Authorization"])
		if err == nil && rtspAuth.Method == headers.AuthBasic {
			credentials.user = rtspAuth.BasicUser
			credentials.pass = rtspAuth.BasicPass
		}
	}

	if externalAuthenticationURL != "" {
		err := doExternalAuthentication(
			externalAuthenticationURL,
			pathName,
			publish,
			credentials,
		)
		if err != nil {
			return &errAuthentication{message: fmt.Sprintf("external authentication failed: %s", err)}
		}
	}

	var pathIPs conf.IPsOrCIDRs
	var pathUser string
	var pathPass string

	if publish {
		pathIPs = pathConf.PublishIPs
		pathUser = string(pathConf.PublishUser)
		pathPass = string(pathConf.PublishPass)
	} else {
		pathIPs = pathConf.ReadIPs
		pathUser = string(pathConf.ReadUser)
		pathPass = string(pathConf.ReadPass)
	}

	if pathIPs != nil {
		if !ipEqualOrInRange(credentials.ip, pathIPs) {
			return &errAuthentication{message: fmt.Sprintf("IP %s not allowed", credentials.ip)}
		}
	}

	if pathUser != "" {
		if credentials.rtspRequest != nil && rtspAuth.Method == headers.AuthDigest {
			err := auth.Validate(
				credentials.rtspRequest,
				pathUser,
				pathPass,
				credentials.rtspBaseURL,
				rtspAuthMethods,
				"IPCAM",
				credentials.rtspNonce)
			if err != nil {
				return &errAuthentication{message: err.Error()}
			}
		} else if !checkCredential(pathUser, credentials.user) ||
			!checkCredential(pathPass, credentials.pass) {
			return &errAuthentication{message: "invalid credentials"}
		}
	}

	return nil
}
