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
	"github.com/bluenviron/mediamtx/internal/defs"
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

func doExternalAuthentication(
	ur string,
	accessRequest defs.PathAccessRequest,
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
		IP:       accessRequest.IP.String(),
		User:     accessRequest.User,
		Password: accessRequest.Pass,
		Path:     accessRequest.Name,
		Protocol: string(accessRequest.Proto),
		ID:       accessRequest.ID,
		Action: func() string {
			if accessRequest.Publish {
				return "publish"
			}
			return "read"
		}(),
		Query: accessRequest.Query,
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
	accessRequest defs.PathAccessRequest,
) error {
	var rtspAuth headers.Authorization
	if accessRequest.RTSPRequest != nil {
		err := rtspAuth.Unmarshal(accessRequest.RTSPRequest.Header["Authorization"])
		if err == nil && rtspAuth.Method == headers.AuthBasic {
			accessRequest.User = rtspAuth.BasicUser
			accessRequest.Pass = rtspAuth.BasicPass
		}
	}

	if externalAuthenticationURL != "" {
		err := doExternalAuthentication(
			externalAuthenticationURL,
			accessRequest,
		)
		if err != nil {
			return &defs.ErrAuthentication{Message: fmt.Sprintf("external authentication failed: %s", err)}
		}
	}

	var pathIPs conf.IPsOrCIDRs
	var pathUser string
	var pathPass string

	if accessRequest.Publish {
		pathIPs = pathConf.PublishIPs
		pathUser = string(pathConf.PublishUser)
		pathPass = string(pathConf.PublishPass)
	} else {
		pathIPs = pathConf.ReadIPs
		pathUser = string(pathConf.ReadUser)
		pathPass = string(pathConf.ReadPass)
	}

	if pathIPs != nil {
		if !ipEqualOrInRange(accessRequest.IP, pathIPs) {
			return &defs.ErrAuthentication{Message: fmt.Sprintf("IP %s not allowed", accessRequest.IP)}
		}
	}

	if pathUser != "" {
		if accessRequest.RTSPRequest != nil && rtspAuth.Method == headers.AuthDigest {
			err := auth.Validate(
				accessRequest.RTSPRequest,
				pathUser,
				pathPass,
				accessRequest.RTSPBaseURL,
				rtspAuthMethods,
				"IPCAM",
				accessRequest.RTSPNonce)
			if err != nil {
				return &defs.ErrAuthentication{Message: err.Error()}
			}
		} else if !checkCredential(pathUser, accessRequest.User) ||
			!checkCredential(pathPass, accessRequest.Pass) {
			return &defs.ErrAuthentication{Message: "invalid credentials"}
		}
	}

	return nil
}
