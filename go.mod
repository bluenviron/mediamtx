module github.com/bluenviron/mediamtx

go 1.21

require (
	code.cloudfoundry.org/bytefmt v0.0.0
	github.com/abema/go-mp4 v1.1.1
	github.com/alecthomas/kong v0.8.1
	github.com/aler9/writerseeker v1.1.0
	github.com/bluenviron/gohlslib v1.0.6
	github.com/bluenviron/gortsplib/v4 v4.6.2
	github.com/bluenviron/mediacommon v1.6.0
	github.com/datarhei/gosrt v0.5.5
	github.com/fsnotify/fsnotify v1.7.0
	github.com/gin-gonic/gin v1.9.1
	github.com/google/uuid v1.5.0
	github.com/gookit/color v1.5.4
	github.com/gorilla/websocket v1.5.1
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/notedit/rtmp v0.0.2
	github.com/pion/ice/v2 v2.3.11
	github.com/pion/interceptor v0.1.25
	github.com/pion/logging v0.2.2
	github.com/pion/rtcp v1.2.13
	github.com/pion/rtp v1.8.3
	github.com/pion/sdp/v3 v3.0.6
	github.com/pion/webrtc/v3 v3.2.22
	github.com/stretchr/testify v1.8.4
	golang.org/x/crypto v0.17.0
	golang.org/x/term v0.15.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/asticode/go-astikit v0.30.0 // indirect
	github.com/asticode/go-astits v1.13.0 // indirect
	github.com/benburkert/openpgp v0.0.0-20160410205803-c2471f86866c // indirect
	github.com/bytedance/sonic v1.9.1 // indirect
	github.com/chenzhuoyu/base64x v0.0.0-20221115062448-fe3a3abad311 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.2 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.14.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.0.8 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.7 // indirect
	github.com/pion/mdns v0.0.9 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.8 // indirect
	github.com/pion/srtp/v2 v2.0.18 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.3 // indirect
	github.com/pion/turn/v2 v2.1.3 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.11 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	golang.org/x/arch v0.3.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace code.cloudfoundry.org/bytefmt => github.com/cloudfoundry/bytefmt v0.0.0-20211005130812-5bb3c17173e5

replace github.com/pion/sdp/v3 => github.com/aler9/sdp/v3 v3.0.0-20231022165400-33437e07f326

replace github.com/pion/ice/v2 => github.com/aler9/ice/v2 v2.0.0-20231112223552-32d34dfcf3a1

replace github.com/pion/webrtc/v3 => github.com/aler9/webrtc/v3 v3.0.0-20231112223655-e402ed2689c6
