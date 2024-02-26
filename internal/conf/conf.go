// Package conf contains the struct that holds the configuration of the software.
package conf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"

	"github.com/bluenviron/mediamtx/internal/conf/decrypt"
	"github.com/bluenviron/mediamtx/internal/conf/env"
	"github.com/bluenviron/mediamtx/internal/conf/yaml"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// ErrPathNotFound is returned when a path is not found.
var ErrPathNotFound = errors.New("path not found")

func sortedKeys(paths map[string]*OptionalPath) []string {
	ret := make([]string, len(paths))
	i := 0
	for name := range paths {
		ret[i] = name
		i++
	}
	sort.Strings(ret)
	return ret
}

func firstThatExists(paths []string) string {
	for _, pa := range paths {
		_, err := os.Stat(pa)
		if err == nil {
			return pa
		}
	}
	return ""
}

func contains(list []headers.AuthMethod, item headers.AuthMethod) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

func copyStructFields(dest interface{}, source interface{}) {
	rvsource := reflect.ValueOf(source).Elem()
	rvdest := reflect.ValueOf(dest)
	nf := rvsource.NumField()
	var zero reflect.Value

	for i := 0; i < nf; i++ {
		fnew := rvsource.Field(i)
		f := rvdest.Elem().FieldByName(rvsource.Type().Field(i).Name)
		if f == zero {
			continue
		}

		if fnew.Kind() == reflect.Pointer {
			if !fnew.IsNil() {
				if f.Kind() == reflect.Ptr {
					f.Set(fnew)
				} else {
					f.Set(fnew.Elem())
				}
			}
		} else {
			f.Set(fnew)
		}
	}
}

// Conf is a configuration.
type Conf struct {
	// General
	LogLevel                  LogLevel        `json:"logLevel"`
	LogDestinations           LogDestinations `json:"logDestinations"`
	LogFile                   string          `json:"logFile"`
	ReadTimeout               StringDuration  `json:"readTimeout"`
	WriteTimeout              StringDuration  `json:"writeTimeout"`
	ReadBufferCount           *int            `json:"readBufferCount,omitempty"` // deprecated
	WriteQueueSize            int             `json:"writeQueueSize"`
	UDPMaxPayloadSize         int             `json:"udpMaxPayloadSize"`
	ExternalAuthenticationURL string          `json:"externalAuthenticationURL"`
	Metrics                   bool            `json:"metrics"`
	MetricsAddress            string          `json:"metricsAddress"`
	PPROF                     bool            `json:"pprof"`
	PPROFAddress              string          `json:"pprofAddress"`
	RunOnConnect              string          `json:"runOnConnect"`
	RunOnConnectRestart       bool            `json:"runOnConnectRestart"`
	RunOnDisconnect           string          `json:"runOnDisconnect"`

	// API
	API        bool   `json:"api"`
	APIAddress string `json:"apiAddress"`

	// Playback
	Playback        bool   `json:"playback"`
	PlaybackAddress string `json:"playbackAddress"`

	// RTSP server
	RTSP              bool             `json:"rtsp"`
	RTSPDisable       *bool            `json:"rtspDisable,omitempty"` // deprecated
	Protocols         Protocols        `json:"protocols"`
	Encryption        Encryption       `json:"encryption"`
	RTSPAddress       string           `json:"rtspAddress"`
	RTSPSAddress      string           `json:"rtspsAddress"`
	RTPAddress        string           `json:"rtpAddress"`
	RTCPAddress       string           `json:"rtcpAddress"`
	MulticastIPRange  string           `json:"multicastIPRange"`
	MulticastRTPPort  int              `json:"multicastRTPPort"`
	MulticastRTCPPort int              `json:"multicastRTCPPort"`
	ServerKey         string           `json:"serverKey"`
	ServerCert        string           `json:"serverCert"`
	AuthMethods       *RTSPAuthMethods `json:"authMethods,omitempty"` // deprecated
	RTSPAuthMethods   RTSPAuthMethods  `json:"rtspAuthMethods"`

	// RTMP server
	RTMP           bool       `json:"rtmp"`
	RTMPDisable    *bool      `json:"rtmpDisable,omitempty"` // deprecated
	RTMPAddress    string     `json:"rtmpAddress"`
	RTMPEncryption Encryption `json:"rtmpEncryption"`
	RTMPSAddress   string     `json:"rtmpsAddress"`
	RTMPServerKey  string     `json:"rtmpServerKey"`
	RTMPServerCert string     `json:"rtmpServerCert"`

	// HLS server
	HLS                bool           `json:"hls"`
	HLSDisable         *bool          `json:"hlsDisable,omitempty"` // depreacted
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
	HLSTrustedProxies  IPNetworks     `json:"hlsTrustedProxies"`
	HLSDirectory       string         `json:"hlsDirectory"`

	// WebRTC server
	WebRTC                      bool              `json:"webrtc"`
	WebRTCDisable               *bool             `json:"webrtcDisable,omitempty"` // deprecated
	WebRTCAddress               string            `json:"webrtcAddress"`
	WebRTCEncryption            bool              `json:"webrtcEncryption"`
	WebRTCServerKey             string            `json:"webrtcServerKey"`
	WebRTCServerCert            string            `json:"webrtcServerCert"`
	WebRTCAllowOrigin           string            `json:"webrtcAllowOrigin"`
	WebRTCTrustedProxies        IPNetworks        `json:"webrtcTrustedProxies"`
	WebRTCLocalUDPAddress       string            `json:"webrtcLocalUDPAddress"`
	WebRTCLocalTCPAddress       string            `json:"webrtcLocalTCPAddress"`
	WebRTCIPsFromInterfaces     bool              `json:"webrtcIPsFromInterfaces"`
	WebRTCIPsFromInterfacesList []string          `json:"webrtcIPsFromInterfacesList"`
	WebRTCAdditionalHosts       []string          `json:"webrtcAdditionalHosts"`
	WebRTCICEServers2           []WebRTCICEServer `json:"webrtcICEServers2"`
	WebRTCICEUDPMuxAddress      *string           `json:"webrtcICEUDPMuxAddress,omitempty"`  // deprecated
	WebRTCICETCPMuxAddress      *string           `json:"webrtcICETCPMuxAddress,omitempty"`  // deprecated
	WebRTCICEHostNAT1To1IPs     *[]string         `json:"webrtcICEHostNAT1To1IPs,omitempty"` // deprecated
	WebRTCICEServers            *[]string         `json:"webrtcICEServers,omitempty"`        // deprecated

	// SRT server
	SRT        bool   `json:"srt"`
	SRTAddress string `json:"srtAddress"`

	// Record (deprecated)
	Record                *bool           `json:"record,omitempty"`                // deprecated
	RecordPath            *string         `json:"recordPath,omitempty"`            // deprecated
	RecordFormat          *RecordFormat   `json:"recordFormat,omitempty"`          // deprecated
	RecordPartDuration    *StringDuration `json:"recordPartDuration,omitempty"`    // deprecated
	RecordSegmentDuration *StringDuration `json:"recordSegmentDuration,omitempty"` // deprecated
	RecordDeleteAfter     *StringDuration `json:"recordDeleteAfter,omitempty"`     // deprecated

	// Path defaults
	PathDefaults Path `json:"pathDefaults"`

	// Paths
	OptionalPaths map[string]*OptionalPath `json:"paths"`
	Paths         map[string]*Path         `json:"-"` // filled by Check()
}

func (conf *Conf) setDefaults() {
	// General
	conf.LogLevel = LogLevel(logger.Info)
	conf.LogDestinations = LogDestinations{logger.DestinationStdout}
	conf.LogFile = "mediamtx.log"
	conf.ReadTimeout = 10 * StringDuration(time.Second)
	conf.WriteTimeout = 10 * StringDuration(time.Second)
	conf.WriteQueueSize = 512
	conf.UDPMaxPayloadSize = 1472
	conf.MetricsAddress = "127.0.0.1:9998"
	conf.PPROFAddress = "127.0.0.1:9999"

	// API
	conf.APIAddress = "127.0.0.1:9997"

	// Playback server
	conf.PlaybackAddress = ":9996"

	// RTSP server
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
	conf.RTSPAuthMethods = RTSPAuthMethods{headers.AuthBasic}

	// RTMP server
	conf.RTMP = true
	conf.RTMPAddress = ":1935"
	conf.RTMPSAddress = ":1936"
	conf.RTMPServerKey = "server.key"
	conf.RTMPServerCert = "server.crt"

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

	// WebRTC server
	conf.WebRTC = true
	conf.WebRTCAddress = ":8889"
	conf.WebRTCServerKey = "server.key"
	conf.WebRTCServerCert = "server.crt"
	conf.WebRTCAllowOrigin = "*"
	conf.WebRTCLocalUDPAddress = ":8189"
	conf.WebRTCIPsFromInterfaces = true
	conf.WebRTCIPsFromInterfacesList = []string{}
	conf.WebRTCAdditionalHosts = []string{}
	conf.WebRTCICEServers2 = []WebRTCICEServer{}

	// SRT server
	conf.SRT = true
	conf.SRTAddress = ":8890"

	conf.PathDefaults.setDefaults()
}

// Load loads a Conf.
func Load(fpath string, defaultConfPaths []string) (*Conf, string, error) {
	conf := &Conf{}

	fpath, err := conf.loadFromFile(fpath, defaultConfPaths)
	if err != nil {
		return nil, "", err
	}

	err = env.Load("RTSP", conf) // legacy prefix
	if err != nil {
		return nil, "", err
	}

	err = env.Load("MTX", conf)
	if err != nil {
		return nil, "", err
	}

	err = conf.Validate()
	if err != nil {
		return nil, "", err
	}

	return conf, fpath, nil
}

func (conf *Conf) loadFromFile(fpath string, defaultConfPaths []string) (string, error) {
	if fpath == "" {
		fpath = firstThatExists(defaultConfPaths)

		// when the configuration file is not explicitly set,
		// it is optional.
		if fpath == "" {
			conf.setDefaults()
			return "", nil
		}
	}

	byts, err := os.ReadFile(fpath)
	if err != nil {
		return "", err
	}

	if key, ok := os.LookupEnv("RTSP_CONFKEY"); ok { // legacy format
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return "", err
		}
	}

	if key, ok := os.LookupEnv("MTX_CONFKEY"); ok {
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return "", err
		}
	}

	err = yaml.Load(byts, conf)
	if err != nil {
		return "", err
	}

	return fpath, nil
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

// Validate checks the configuration for errors.
func (conf *Conf) Validate() error {
	// General

	if conf.ReadBufferCount != nil {
		conf.WriteQueueSize = *conf.ReadBufferCount
	}
	if (conf.WriteQueueSize & (conf.WriteQueueSize - 1)) != 0 {
		return fmt.Errorf("'writeQueueSize' must be a power of two")
	}
	if conf.UDPMaxPayloadSize > 1472 {
		return fmt.Errorf("'udpMaxPayloadSize' must be less than 1472")
	}
	if conf.ExternalAuthenticationURL != "" {
		if !strings.HasPrefix(conf.ExternalAuthenticationURL, "http://") &&
			!strings.HasPrefix(conf.ExternalAuthenticationURL, "https://") {
			return fmt.Errorf("'externalAuthenticationURL' must be a HTTP URL")
		}
	}

	// RTSP

	if conf.RTSPDisable != nil {
		conf.RTSP = !*conf.RTSPDisable
	}
	if conf.Encryption == EncryptionStrict {
		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDP)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP transport protocol")
		}
		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDPMulticast)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP-multicast transport protocol")
		}
	}
	if conf.AuthMethods != nil {
		conf.RTSPAuthMethods = *conf.AuthMethods
	}
	if conf.ExternalAuthenticationURL != "" && contains(conf.RTSPAuthMethods, headers.AuthDigestMD5) {
		return fmt.Errorf("'externalAuthenticationURL' can't be used when 'digest' is in authMethods")
	}

	// RTMP

	if conf.RTMPDisable != nil {
		conf.RTMP = !*conf.RTMPDisable
	}

	// HLS

	if conf.HLSDisable != nil {
		conf.HLS = !*conf.HLSDisable
	}

	// WebRTC

	if conf.WebRTCDisable != nil {
		conf.WebRTC = !*conf.WebRTCDisable
	}
	if conf.WebRTCICEUDPMuxAddress != nil {
		conf.WebRTCLocalUDPAddress = *conf.WebRTCICEUDPMuxAddress
	}
	if conf.WebRTCICETCPMuxAddress != nil {
		conf.WebRTCLocalTCPAddress = *conf.WebRTCICETCPMuxAddress
	}
	if conf.WebRTCICEHostNAT1To1IPs != nil {
		conf.WebRTCAdditionalHosts = *conf.WebRTCICEHostNAT1To1IPs
	}
	if conf.WebRTCICEServers != nil {
		for _, server := range *conf.WebRTCICEServers {
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
	}
	for _, server := range conf.WebRTCICEServers2 {
		if !strings.HasPrefix(server.URL, "stun:") &&
			!strings.HasPrefix(server.URL, "turn:") &&
			!strings.HasPrefix(server.URL, "turns:") {
			return fmt.Errorf("invalid ICE server: '%s'", server.URL)
		}
	}
	if conf.WebRTCLocalUDPAddress == "" &&
		conf.WebRTCLocalTCPAddress == "" &&
		len(conf.WebRTCICEServers2) == 0 {
		return fmt.Errorf("at least one between 'webrtcLocalUDPAddress'," +
			" 'webrtcLocalTCPAddress' or 'webrtcICEServers2' must be filled")
	}
	if conf.WebRTCLocalUDPAddress != "" || conf.WebRTCLocalTCPAddress != "" {
		if !conf.WebRTCIPsFromInterfaces && len(conf.WebRTCAdditionalHosts) == 0 {
			return fmt.Errorf("at least one between 'webrtcIPsFromInterfaces' or 'webrtcAdditionalHosts' must be filled")
		}
	}

	// Record (deprecated)
	if conf.Record != nil {
		conf.PathDefaults.Record = *conf.Record
	}
	if conf.RecordPath != nil {
		conf.PathDefaults.RecordPath = *conf.RecordPath
	}
	if conf.RecordFormat != nil {
		conf.PathDefaults.RecordFormat = *conf.RecordFormat
	}
	if conf.RecordPartDuration != nil {
		conf.PathDefaults.RecordPartDuration = *conf.RecordPartDuration
	}
	if conf.RecordSegmentDuration != nil {
		conf.PathDefaults.RecordSegmentDuration = *conf.RecordSegmentDuration
	}
	if conf.RecordDeleteAfter != nil {
		conf.PathDefaults.RecordDeleteAfter = *conf.RecordDeleteAfter
	}

	hasAllOthers := false
	for name := range conf.OptionalPaths {
		if name == "all" || name == "all_others" || name == "~^.*$" {
			if hasAllOthers {
				return fmt.Errorf("all_others, all and '~^.*$' are aliases")
			}
			hasAllOthers = true
		}
	}

	conf.Paths = make(map[string]*Path)

	for _, name := range sortedKeys(conf.OptionalPaths) {
		optional := conf.OptionalPaths[name]
		if optional == nil {
			optional = &OptionalPath{
				Values: newOptionalPathValues(),
			}
		}

		pconf := newPath(&conf.PathDefaults, optional)
		conf.Paths[name] = pconf

		err := pconf.validate(conf, name)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (conf *Conf) UnmarshalJSON(b []byte) error {
	conf.setDefaults()

	type alias Conf
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*alias)(conf))
}

// Global returns the global part of Conf.
func (conf *Conf) Global() *Global {
	g := &Global{
		Values: newGlobalValues(),
	}
	copyStructFields(g.Values, conf)
	return g
}

// PatchGlobal patches the global configuration.
func (conf *Conf) PatchGlobal(optional *OptionalGlobal) {
	copyStructFields(conf, optional.Values)
}

// PatchPathDefaults patches path default settings.
func (conf *Conf) PatchPathDefaults(optional *OptionalPath) {
	copyStructFields(&conf.PathDefaults, optional.Values)
}

// AddPath adds a path.
func (conf *Conf) AddPath(name string, p *OptionalPath) error {
	if _, ok := conf.OptionalPaths[name]; ok {
		return fmt.Errorf("path already exists")
	}

	if conf.OptionalPaths == nil {
		conf.OptionalPaths = make(map[string]*OptionalPath)
	}

	conf.OptionalPaths[name] = p
	return nil
}

// PatchPath patches a path.
func (conf *Conf) PatchPath(name string, optional2 *OptionalPath) error {
	optional, ok := conf.OptionalPaths[name]
	if !ok {
		return ErrPathNotFound
	}

	copyStructFields(optional.Values, optional2.Values)
	return nil
}

// ReplacePath replaces a path.
func (conf *Conf) ReplacePath(name string, optional2 *OptionalPath) error {
	_, ok := conf.OptionalPaths[name]
	if !ok {
		return ErrPathNotFound
	}

	conf.OptionalPaths[name] = optional2
	return nil
}

// RemovePath removes a path.
func (conf *Conf) RemovePath(name string) error {
	if _, ok := conf.OptionalPaths[name]; !ok {
		return ErrPathNotFound
	}

	delete(conf.OptionalPaths, name)
	return nil
}
