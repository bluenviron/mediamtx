package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
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

func doExternalAuthentication(
	ur string,
	accessRequest pathAccessRequest,
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
		IP:       accessRequest.ip.String(),
		User:     accessRequest.user,
		Password: accessRequest.pass,
		Path:     accessRequest.name,
		Protocol: string(accessRequest.proto),
		ID:       accessRequest.id,
		Action: func() string {
			if accessRequest.publish {
				return "publish"
			}
			return "read"
		}(),
		Query: accessRequest.query,
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
	pathConf *conf.Path,
	accessRequest pathAccessRequest,
) error {
	var rtspAuth headers.Authorization
	if accessRequest.rtspRequest != nil {
		err := rtspAuth.Unmarshal(accessRequest.rtspRequest.Header["Authorization"])
		if err == nil && rtspAuth.Method == headers.AuthBasic {
			accessRequest.user = rtspAuth.BasicUser
			accessRequest.pass = rtspAuth.BasicPass
		}
	}

	if externalAuthenticationURL != "" {
		err := doExternalAuthentication(
			externalAuthenticationURL,
			accessRequest,
		)
		if err != nil {
			return &errAuthentication{message: fmt.Sprintf("external authentication failed: %s", err)}
		}
	}

	var pathIPs conf.IPsOrCIDRs
	var pathUser string
	var pathPass string

	if accessRequest.publish {
		pathIPs = pathConf.PublishIPs
		pathUser = string(pathConf.PublishUser)
		pathPass = string(pathConf.PublishPass)
	} else {
		pathIPs = pathConf.ReadIPs
		pathUser = string(pathConf.ReadUser)
		pathPass = string(pathConf.ReadPass)
	}

	if pathIPs != nil {
		if !ipEqualOrInRange(accessRequest.ip, pathIPs) {
			return &errAuthentication{message: fmt.Sprintf("IP %s not allowed", accessRequest.ip)}
		}
	}

	if pathUser != "" {
		if accessRequest.rtspRequest != nil && rtspAuth.Method == headers.AuthDigest {
			err := auth.Validate(
				accessRequest.rtspRequest,
				pathUser,
				pathPass,
				accessRequest.rtspBaseURL,
				rtspAuthMethods,
				"IPCAM",
				accessRequest.rtspNonce)
			if err != nil {
				return &errAuthentication{message: err.Error()}
			}
		} else if !checkCredential(pathUser, accessRequest.user) ||
			!checkCredential(pathPass, accessRequest.pass) {
			return &errAuthentication{message: "invalid credentials"}
		}
	}

	return nil
}
