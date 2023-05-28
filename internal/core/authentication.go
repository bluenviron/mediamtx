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

type authProtocol string

const (
	authProtocolRTSP   authProtocol = "rtsp"
	authProtocolRTMP   authProtocol = "rtmp"
	authProtocolHLS    authProtocol = "hls"
	authProtocolWebRTC authProtocol = "webrtc"
)

func externalAuth(
	ur string,
	ip string,
	user string,
	password string,
	path string,
	protocol authProtocol,
	id *uuid.UUID,
	publish bool,
	query string,
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
		IP:       ip,
		User:     user,
		Password: password,
		Path:     path,
		Protocol: string(protocol),
		ID:       id,
		Action: func() string {
			if publish {
				return "publish"
			}
			return "read"
		}(),
		Query: query,
	})
	res, err := http.Post(ur, "application/json", bytes.NewReader(enc))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		if resBody, err := io.ReadAll(res.Body); err == nil && len(resBody) != 0 {
			return fmt.Errorf("external authentication replied with code %d: %s", res.StatusCode, string(resBody))
		}

		return fmt.Errorf("external authentication replied with code %d", res.StatusCode)
	}

	return nil
}

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

func authenticate(
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
		err := externalAuth(
			externalAuthenticationURL,
			credentials.ip.String(),
			credentials.user,
			credentials.pass,
			pathName,
			credentials.proto,
			credentials.id,
			publish,
			credentials.query,
		)
		if err != nil {
			return fmt.Errorf("external authentication failed: %s", err)
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
			return fmt.Errorf("IP '%s' not allowed", credentials.ip)
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
				return err
			}
		} else if !checkCredential(pathUser, credentials.user) ||
			!checkCredential(pathPass, credentials.pass) {
			return fmt.Errorf("invalid credentials")
		}
	}

	return nil
}
