package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type externalAuthProto string

const (
	externalAuthProtoRTSP   externalAuthProto = "rtsp"
	externalAuthProtoRTMP   externalAuthProto = "rtmp"
	externalAuthProtoHLS    externalAuthProto = "hls"
	externalAuthProtoWebRTC externalAuthProto = "webrtc"
)

func externalAuth(
	ur string,
	ip string,
	user string,
	password string,
	path string,
	protocol externalAuthProto,
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
		return fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	return nil
}
