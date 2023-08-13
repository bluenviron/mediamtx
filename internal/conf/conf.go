// Package conf contains the struct that holds the configuration of the software.
package conf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/headers"

	"github.com/bluenviron/mediamtx/internal/conf/decrypt"
	"github.com/bluenviron/mediamtx/internal/conf/env"
	"github.com/bluenviron/mediamtx/internal/conf/yaml"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func getSortedKeys(paths map[string]*PathConf) []string {
	ret := make([]string, len(paths))
	i := 0
	for name := range paths {
		ret[i] = name
		i++
	}
	sort.Strings(ret)
	return ret
}

func loadFromFile(fpath string, conf *Conf) (bool, error) {
	if fpath == "mediamtx.yml" {
		// give priority to the legacy configuration file, in order not to break
		// existing setups
		if _, err := os.Stat("rtsp-simple-server.yml"); err == nil {
			fpath = "rtsp-simple-server.yml"
		}
	}

	// mediamtx.yml is optional
	// other configuration files are not
	if fpath == "mediamtx.yml" || fpath == "rtsp-simple-server.yml" {
		if _, err := os.Stat(fpath); errors.Is(err, os.ErrNotExist) {
			// load defaults
			conf.UnmarshalJSON(nil) //nolint:errcheck
			return false, nil
		}
	}

	byts, err := os.ReadFile(fpath)
	if err != nil {
		return true, err
	}

	if key, ok := os.LookupEnv("RTSP_CONFKEY"); ok { // legacy format
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return true, err
		}
	}

	if key, ok := os.LookupEnv("MTX_CONFKEY"); ok {
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return true, err
		}
	}

	err = yaml.Load(byts, conf)
	if err != nil {
		return true, err
	}

	return true, nil
}

func contains(list []headers.AuthMethod, item headers.AuthMethod) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

// Conf is a configuration.
type Conf struct {
	// general
	LogLevel                  LogLevel        `json:"logLevel"`
	LogDestinations           LogDestinations `json:"logDestinations"`
	LogFile                   string          `json:"logFile"`
	ReadTimeout               StringDuration  `json:"readTimeout"`
	WriteTimeout              StringDuration  `json:"writeTimeout"`
	ReadBufferCount           int             `json:"readBufferCount"`
	UDPMaxPayloadSize         int             `json:"udpMaxPayloadSize"`
	ExternalAuthenticationURL string          `json:"externalAuthenticationURL"`
	API                       bool            `json:"api"`
	APIAddress                string          `json:"apiAddress"`
	Metrics                   bool            `json:"metrics"`
	MetricsAddress            string          `json:"metricsAddress"`
	PPROF                     bool            `json:"pprof"`
	PPROFAddress              string          `json:"pprofAddress"`
	RunOnConnect              string          `json:"runOnConnect"`
	RunOnConnectRestart       bool            `json:"runOnConnectRestart"`

	// RTSP
	RTSP              bool        `json:"rtsp"`
	RTSPDisable       bool        `json:"rtspDisable"` // deprecated
	Protocols         Protocols   `json:"protocols"`
	Encryption        Encryption  `json:"encryption"`
	RTSPAddress       string      `json:"rtspAddress"`
	RTSPSAddress      string      `json:"rtspsAddress"`
	RTPAddress        string      `json:"rtpAddress"`
	RTCPAddress       string      `json:"rtcpAddress"`
	MulticastIPRange  string      `json:"multicastIPRange"`
	MulticastRTPPort  int         `json:"multicastRTPPort"`
	MulticastRTCPPort int         `json:"multicastRTCPPort"`
	ServerKey         string      `json:"serverKey"`
	ServerCert        string      `json:"serverCert"`
	AuthMethods       AuthMethods `json:"authMethods"`

	// RTMP
	RTMP           bool       `json:"rtmp"`
	RTMPDisable    bool       `json:"rtmpDisable"` // deprecated
	RTMPAddress    string     `json:"rtmpAddress"`
	RTMPEncryption Encryption `json:"rtmpEncryption"`
	RTMPSAddress   string     `json:"rtmpsAddress"`
	RTMPServerKey  string     `json:"rtmpServerKey"`
	RTMPServerCert string     `json:"rtmpServerCert"`

	// HLS
	HLS                bool           `json:"hls"`
	HLSDisable         bool           `json:"hlsDisable"` // depreacted
	HLSAddress         string         `json:"hlsAddress"`
	HLSEncryption      bool           `json:"hlsEncryption"`
	HLSServerKey       string         `json:"hlsServerKey"`
	HLSServerCert      string         `json:"hlsServerCert"`
	HLSAlwaysRemux     bool           `json:"hlsAlwaysRemux"`
	HLSVariant         HLSVariant     `json:"hlsVariant"`
	HLSSegmentCount    int            `json:"hlsSegmentCount"`
	HLSSegmentDuration StringDuration `json:"hlsSegmentDuration"`
	HLSPartDuration    StringDuration `json:"hlsPartDuration"`
	HLSSegmentMaxSize  StringSize     `json:"hlsSegmentMaxSize"`
	HLSAllowOrigin     string         `json:"hlsAllowOrigin"`
	HLSTrustedProxies  IPsOrCIDRs     `json:"hlsTrustedProxies"`
	HLSDirectory       string         `json:"hlsDirectory"`

	// WebRTC
	WebRTC                  bool              `json:"webrtc"`
	WebRTCDisable           bool              `json:"webrtcDisable"` // deprecated
	WebRTCAddress           string            `json:"webrtcAddress"`
	WebRTCEncryption        bool              `json:"webrtcEncryption"`
	WebRTCServerKey         string            `json:"webrtcServerKey"`
	WebRTCServerCert        string            `json:"webrtcServerCert"`
	WebRTCAllowOrigin       string            `json:"webrtcAllowOrigin"`
	WebRTCTrustedProxies    IPsOrCIDRs        `json:"webrtcTrustedProxies"`
	WebRTCICEServers        []string          `json:"webrtcICEServers"` // deprecated
	WebRTCICEServers2       []WebRTCICEServer `json:"webrtcICEServers2"`
	WebRTCICEHostNAT1To1IPs []string          `json:"webrtcICEHostNAT1To1IPs"`
	WebRTCICEUDPMuxAddress  string            `json:"webrtcICEUDPMuxAddress"`
	WebRTCICETCPMuxAddress  string            `json:"webrtcICETCPMuxAddress"`

	// SRT
	SRT        bool   `json:"srt"`
	SRTAddress string `json:"srtAddress"`

	// paths
	Paths map[string]*PathConf `json:"paths"`
}

// Load loads a Conf.
func Load(fpath string) (*Conf, bool, error) {
	conf := &Conf{}

	found, err := loadFromFile(fpath, conf)
	if err != nil {
		return nil, false, err
	}

	err = env.Load("RTSP", conf) // legacy prefix
	if err != nil {
		return nil, false, err
	}

	err = env.Load("MTX", conf)
	if err != nil {
		return nil, false, err
	}

	err = conf.Check()
	if err != nil {
		return nil, false, err
	}

	return conf, found, nil
}

// Clone clones the configuration.
func (conf Conf) Clone() *Conf {
	enc, err := json.Marshal(conf)
	if err != nil {
		panic(err)
	}

	var dest Conf
	err = json.Unmarshal(enc, &dest)
	if err != nil {
		panic(err)
	}

	return &dest
}

// Check checks the configuration for errors.
func (conf *Conf) Check() error {
	// general
	if (conf.ReadBufferCount & (conf.ReadBufferCount - 1)) != 0 {
		return fmt.Errorf("'readBufferCount' must be a power of two")
	}
	if conf.UDPMaxPayloadSize > 1472 {
		return fmt.Errorf("'udpMaxPayloadSize' must be less than 1472")
	}
	if conf.ExternalAuthenticationURL != "" {
		if !strings.HasPrefix(conf.ExternalAuthenticationURL, "http://") &&
			!strings.HasPrefix(conf.ExternalAuthenticationURL, "https://") {
			return fmt.Errorf("'externalAuthenticationURL' must be a HTTP URL")
		}

		if contains(conf.AuthMethods, headers.AuthDigest) {
			return fmt.Errorf("'externalAuthenticationURL' can't be used when 'digest' is in authMethods")
		}
	}

	// RTSP
	if conf.RTSPDisable {
		conf.RTSP = false
	}
	if conf.Encryption == EncryptionStrict {
		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDP)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP transport protocol")
		}
		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDPMulticast)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP-multicast transport protocol")
		}
	}

	// RTMP
	if conf.RTMPDisable {
		conf.RTMP = false
	}

	// HLS
	if conf.HLSDisable {
		conf.HLS = false
	}

	// WebRTC
	if conf.WebRTCDisable {
		conf.WebRTC = false
	}
	for _, server := range conf.WebRTCICEServers {
		parts := strings.Split(server, ":")
		if len(parts) == 5 {
			conf.WebRTCICEServers2 = append(conf.WebRTCICEServers2, WebRTCICEServer{
				URL:      parts[0] + ":" + parts[3] + ":" + parts[4],
				Username: parts[1],
				Password: parts[2],
			})
		} else {
			conf.WebRTCICEServers2 = append(conf.WebRTCICEServers2, WebRTCICEServer{
				URL: server,
			})
		}
	}
	conf.WebRTCICEServers = nil
	for _, server := range conf.WebRTCICEServers2 {
		if !strings.HasPrefix(server.URL, "stun:") &&
			!strings.HasPrefix(server.URL, "turn:") &&
			!strings.HasPrefix(server.URL, "turns:") {
			return fmt.Errorf("invalid ICE server: '%s'", server.URL)
		}
	}

	// do not add automatically "all", since user may want to
	// initialize all paths through API or hot reloading.
	if conf.Paths == nil {
		conf.Paths = make(map[string]*PathConf)
	}

	for _, name := range getSortedKeys(conf.Paths) {
		pconf := conf.Paths[name]
		if pconf == nil {
			pconf = &PathConf{}
			// load defaults
			pconf.UnmarshalJSON(nil) //nolint:errcheck
			conf.Paths[name] = pconf
		}

		err := pconf.check(conf, name)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalJSON implements json.Unmarshaler. It is used to set default values.
func (conf *Conf) UnmarshalJSON(b []byte) error {
	// general
	conf.LogLevel = LogLevel(logger.Info)
	conf.LogDestinations = LogDestinations{logger.DestinationStdout}
	conf.LogFile = "mediamtx.log"
	conf.ReadTimeout = 10 * StringDuration(time.Second)
	conf.WriteTimeout = 10 * StringDuration(time.Second)
	conf.ReadBufferCount = 512
	conf.UDPMaxPayloadSize = 1472
	conf.APIAddress = "127.0.0.1:9997"
	conf.MetricsAddress = "127.0.0.1:9998"
	conf.PPROFAddress = "127.0.0.1:9999"

	// RTSP
	conf.RTSP = true
	conf.Protocols = Protocols{
		Protocol(gortsplib.TransportUDP):          {},
		Protocol(gortsplib.TransportUDPMulticast): {},
		Protocol(gortsplib.TransportTCP):          {},
	}
	conf.RTSPAddress = ":8554"
	conf.RTSPSAddress = ":8322"
	conf.RTPAddress = ":8000"
	conf.RTCPAddress = ":8001"
	conf.MulticastIPRange = "224.1.0.0/16"
	conf.MulticastRTPPort = 8002
	conf.MulticastRTCPPort = 8003
	conf.ServerKey = "server.key"
	conf.ServerCert = "server.crt"
	conf.AuthMethods = AuthMethods{headers.AuthBasic}

	// RTMP
	conf.RTMP = true
	conf.RTMPAddress = ":1935"
	conf.RTMPSAddress = ":1936"

	// HLS
	conf.HLS = true
	conf.HLSAddress = ":8888"
	conf.HLSServerKey = "server.key"
	conf.HLSServerCert = "server.crt"
	conf.HLSVariant = HLSVariant(gohlslib.MuxerVariantLowLatency)
	conf.HLSSegmentCount = 7
	conf.HLSSegmentDuration = 1 * StringDuration(time.Second)
	conf.HLSPartDuration = 200 * StringDuration(time.Millisecond)
	conf.HLSSegmentMaxSize = 50 * 1024 * 1024
	conf.HLSAllowOrigin = "*"

	// WebRTC
	conf.WebRTC = true
	conf.WebRTCAddress = ":8889"
	conf.WebRTCServerKey = "server.key"
	conf.WebRTCServerCert = "server.crt"
	conf.WebRTCAllowOrigin = "*"
	conf.WebRTCICEServers2 = []WebRTCICEServer{{URL: "stun:stun.l.google.com:19302"}}

	// SRT
	conf.SRT = true
	conf.SRTAddress = ":8890"

	type alias Conf
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*alias)(conf))
}
