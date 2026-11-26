package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/api"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestConfigGlobalGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"authInternalUsers:\n"+
		"  - user: myuser\n"+
		"    pass: mypass\n"+
		"    permissions:\n"+
		"      - action: api\n")
	checked := false

	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) (*auth.Result, *auth.Error) {
				require.Equal(t, conf.AuthActionAPI, req.Action)
				require.Equal(t, "myuser", req.Credentials.User)
				require.Equal(t, "mypass", req.Credentials.Pass)
				checked = true
				return &auth.Result{User: req.Credentials.User}, nil
			},
		},
		Parent: &testParent{conf: cnf},
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://myuser:mypass@localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, true, out["api"])

	authInternalUsers := out["authInternalUsers"].([]any)
	require.Len(t, authInternalUsers, 1)
	require.Equal(t, "<redacted>", authInternalUsers[0].(map[string]any)["pass"])

	require.True(t, checked)
}

func TestConfigGlobalPatch(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{conf: cnf},
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		map[string]any{
			"rtmp":            false,
			"readTimeout":     "7s",
			"protocols":       []string{"tcp"},
			"readBufferCount": 4096, // test setting a deprecated parameter
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, false, out["rtmp"])
	require.Equal(t, "7s", out["readTimeout"])
	require.Equal(t, []any{"tcp"}, out["protocols"])
	require.Equal(t, float64(4096), out["readBufferCount"])
}

func TestConfigGlobalPatchRedactedPasswords(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"authInternalUsers:\n"+
		"  - user: alice\n"+
		"    pass: alicepass\n"+
		"    permissions:\n"+
		"      - action: api\n"+
		"  - user: bob\n"+
		"    pass: bobpass\n"+
		"    permissions:\n"+
		"      - action: api\n")

	parent := &testParent{conf: cnf}
	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       parent,
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	users := out["authInternalUsers"].([]any)
	require.Equal(t, "<redacted>", users[0].(map[string]any)["pass"])
	require.Equal(t, "<redacted>", users[1].(map[string]any)["pass"])

	users[0], users[1] = users[1], users[0]
	users[0].(map[string]any)["pass"] = "newbobpass"
	out["readTimeout"] = "7s"
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", out, nil)

	require.Equal(t, conf.Credential("newbobpass"), parent.conf.AuthInternalUsers[0].Pass)
	require.Equal(t, conf.Credential("alicepass"), parent.conf.AuthInternalUsers[1].Pass)
	require.Equal(t, conf.Duration(7*time.Second), parent.conf.ReadTimeout)

	users[1].(map[string]any)["user"] = "charlie"
	byts, err := json.Marshal(out)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPatch, "http://localhost:9997/v3/config/global/patch", bytes.NewReader(byts))
	require.NoError(t, err)
	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	checkError(t, res.Body, "cannot keep redacted password for user \"charlie\": user not found")
	require.Equal(t, conf.Credential("alicepass"), parent.conf.AuthInternalUsers[1].Pass)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		map[string]any{"readTimeout": "8s"}, nil)
	require.Equal(t, conf.Credential("alicepass"), parent.conf.AuthInternalUsers[1].Pass)

	users[1].(map[string]any)["user"] = "charlie"
	users[1].(map[string]any)["pass"] = "charliepass"
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", out, nil)
	require.Equal(t, conf.Credential("charliepass"), parent.conf.AuthInternalUsers[1].Pass)
}

func TestConfigGlobalPatchUnknownField(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{conf: cnf},
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	b := map[string]any{
		"test": "asd",
	}

	byts, err := json.Marshal(b)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		bytes.NewReader(byts))
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	checkError(t, res.Body, "json: unknown field \"test\"")
}

func TestConfigRedactedSecretsRoundTrip(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"hlsCDNSecret: cdnsecret\n"+
		"authHTTPAddress: http://authuser:authpass@127.0.0.1:9120/auth\n"+
		"authInternalUsers:\n"+
		"  - user: any\n"+
		"    permissions:\n"+
		"      - action: api\n"+
		"webrtcICEServers2:\n"+
		"  - url: turn:host:3478\n"+
		"    username: u\n"+
		"    password: turnpass\n"+
		"paths:\n"+
		"  cam:\n"+
		"    source: rtsp://camuser:campass@127.0.0.1:8554/stream\n"+
		"    sourceOnDemand: yes\n"+
		"    whepBearerToken: wheptoken\n"+
		"    forward:\n"+
		"      - dest: rtmp://pubuser:pubpass@host/app\n"+
		"        whipBearerToken: whiptoken\n"+
		"      - dest: rtsps+ws://host/app\n"+
		"        destHeaders:\n"+
		"          X-Secret: headersecret\n"+
		"  srtcam:\n"+
		"    source: srt://127.0.0.1:9000?streamid=read:cam:user:pass&passphrase=secret1234&latency=200\n"+
		"    sourceOnDemand: yes\n"+
		"    forward:\n"+
		"      - dest: srt://host:9001?passphrase=other1234&streamid=publish:x\n")

	parent := &testParent{conf: cnf}
	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       parent,
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var global map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &global)
	require.Equal(t, "<redacted>", global["hlsCDNSecret"])
	require.Equal(t, "http://<redacted>:<redacted>@127.0.0.1:9120/auth", global["authHTTPAddress"])
	require.Equal(t, "<redacted>", global["webrtcICEServers2"].([]any)[0].(map[string]any)["password"])
	require.Equal(t, "<redacted>", global["webrtcICEServers2"].([]any)[0].(map[string]any)["username"])

	var path map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/cam", nil, &path)
	require.Equal(t, "rtsp://<redacted>:<redacted>@127.0.0.1:8554/stream", path["source"])
	require.Equal(t, "<redacted>", path["whepBearerToken"])
	require.Equal(t, "rtmp://<redacted>:<redacted>@host/app", path["forward"].([]any)[0].(map[string]any)["dest"])
	require.Equal(t, map[string]any{"X-Secret": "<redacted>"},
		path["forward"].([]any)[1].(map[string]any)["destHeaders"])

	global["readTimeout"] = "7s"
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", global, nil)
	require.Equal(t, "cdnsecret", parent.conf.HLSCDNSecret)
	require.Equal(t, "http://authuser:authpass@127.0.0.1:9120/auth", parent.conf.AuthHTTPAddress)
	require.Equal(t, "turnpass", parent.conf.WebRTCICEServers2[0].Password)
	require.Equal(t, "u", parent.conf.WebRTCICEServers2[0].Username)

	path["sourceOnDemand"] = false
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/cam", path, nil)
	cam := parent.conf.Paths["cam"]
	require.Equal(t, "rtsp://camuser:campass@127.0.0.1:8554/stream", cam.Source)
	require.Equal(t, "wheptoken", cam.WHEPBearerToken)
	require.Equal(t, "rtmp://pubuser:pubpass@host/app", cam.Forward[0].Dest)
	require.Equal(t, "whiptoken", cam.Forward[0].WHIPBearerToken)
	require.Equal(t, map[string]string{"X-Secret": "headersecret"}, cam.Forward[1].DestHeaders)

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/cam", path, nil)
	cam = parent.conf.Paths["cam"]
	require.Equal(t, "rtsp://camuser:campass@127.0.0.1:8554/stream", cam.Source)
	require.Equal(t, "whiptoken", cam.Forward[0].WHIPBearerToken)

	var srtPath map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/srtcam", nil, &srtPath)
	require.Equal(t, "srt://127.0.0.1:9000?streamid=<redacted>&passphrase=<redacted>&latency=200", srtPath["source"])
	require.Equal(t, "srt://host:9001?passphrase=<redacted>&streamid=<redacted>",
		srtPath["forward"].([]any)[0].(map[string]any)["dest"])

	srtPath["sourceOnDemand"] = false
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/srtcam", srtPath, nil)
	srtCam := parent.conf.Paths["srtcam"]
	require.Equal(t, "srt://127.0.0.1:9000?streamid=read:cam:user:pass&passphrase=secret1234&latency=200",
		srtCam.Source)
	require.Equal(t, "srt://host:9001?passphrase=other1234&streamid=publish:x", srtCam.Forward[0].Dest)

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/srtcam", srtPath, nil)
	srtCam = parent.conf.Paths["srtcam"]
	require.Equal(t, "srt://127.0.0.1:9000?streamid=read:cam:user:pass&passphrase=secret1234&latency=200",
		srtCam.Source)
	require.Equal(t, "srt://host:9001?passphrase=other1234&streamid=publish:x", srtCam.Forward[0].Dest)
}

func TestConfigRedactedLegacyICEServersRoundTrip(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"authInternalUsers:\n"+
		"  - user: any\n"+
		"    permissions:\n"+
		"      - action: api\n"+
		"webrtcICEServers:\n"+
		"  - turn:turnuser:turnpass:host:3478\n"+
		"  - stun:stun.example.com:3478\n")

	parent := &testParent{conf: cnf}
	a := api.API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       parent,
	}
	err := a.Initialize()
	require.NoError(t, err)
	defer a.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var global map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &global)
	require.Equal(t, []any{
		"turn:<redacted>:<redacted>:host:3478",
		"stun:stun.example.com:3478",
	}, global["webrtcICEServers"])

	global["readTimeout"] = "7s"
	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", global, nil)
	require.Equal(t, []string{
		"turn:turnuser:turnpass:host:3478",
		"stun:stun.example.com:3478",
	}, *parent.conf.WebRTCICEServers)
}
