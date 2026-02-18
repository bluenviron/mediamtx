// Package conf contains the struct that holds the configuration of the software.
package conf

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/auth"

	"github.com/bluenviron/mediamtx/internal/conf/decrypt"
	"github.com/bluenviron/mediamtx/internal/conf/env"
	"github.com/bluenviron/mediamtx/internal/conf/yamlwrapper"
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

func setAllNilSlicesToEmptyRecursive(rv reflect.Value) {
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Struct {
		for i := range rv.NumField() {
			field := rv.Field(i)

			switch field.Kind() {
			case reflect.Slice:
				if field.IsNil() {
					field.Set(reflect.MakeSlice(field.Type(), 0, 0))
				} else {
					for j := range field.Len() {
						elem := field.Index(j)
						if elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Struct {
							setAllNilSlicesToEmptyRecursive(elem)
						}
					}
				}

			case reflect.Pointer:
				if !field.IsNil() {
					setAllNilSlicesToEmptyRecursive(field)
				}

			case reflect.Struct:
				setAllNilSlicesToEmptyRecursive(field.Addr())

			case reflect.Map:
				if !field.IsNil() {
					for _, key := range field.MapKeys() {
						mapValue := field.MapIndex(key)
						if mapValue.Kind() == reflect.Pointer {
							setAllNilSlicesToEmptyRecursive(mapValue)
						}
					}
				}
			}
		}
	}
}

func copyStructFields(dest any, source any) {
	rvsource := reflect.ValueOf(source).Elem()
	rvdest := reflect.ValueOf(dest)
	nf := rvsource.NumField()
	var zero reflect.Value

	for i := range nf {
		fnew := rvsource.Field(i)
		f := rvdest.Elem().FieldByName(rvsource.Type().Field(i).Name)
		if f == zero {
			continue
		}

		if fnew.Kind() == reflect.Pointer {
			if !fnew.IsNil() {
				if f.Kind() == reflect.Pointer {
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

func mustParseCIDR(v string) IPNetwork {
	_, ne, err := net.ParseCIDR(v)
	if err != nil {
		panic(err)
	}
	if ipv4 := ne.IP.To4(); ipv4 != nil {
		return IPNetwork{IP: ipv4, Mask: ne.Mask[len(ne.Mask)-4 : len(ne.Mask)]}
	}
	return IPNetwork(*ne)
}

func anyPathHasDeprecatedCredentials(pathDefaults Path, paths map[string]*OptionalPath) bool {
	if pathDefaults.PublishUser != nil ||
		pathDefaults.PublishPass != nil ||
		pathDefaults.PublishIPs != nil ||
		pathDefaults.ReadUser != nil ||
		pathDefaults.ReadPass != nil ||
		pathDefaults.ReadIPs != nil {
		return true
	}

	for _, pa := range paths {
		if pa != nil {
			rva := reflect.ValueOf(pa.Values).Elem()
			if rva.FieldByName("PublishUser").Interface().(*Credential) != nil ||
				rva.FieldByName("PublishPass").Interface().(*Credential) != nil ||
				rva.FieldByName("PublishIPs").Interface().(*IPNetworks) != nil ||
				rva.FieldByName("ReadUser").Interface().(*Credential) != nil ||
				rva.FieldByName("ReadPass").Interface().(*Credential) != nil ||
				rva.FieldByName("ReadIPs").Interface().(*IPNetworks) != nil {
				return true
			}
		}
	}
	return false
}

func deepClone(rv reflect.Value) reflect.Value {
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return rv
		}
		newPtr := reflect.New(rv.Elem().Type())
		newPtr.Elem().Set(deepClone(rv.Elem()))
		return newPtr

	case reflect.Struct:
		newStruct := reflect.New(rv.Type()).Elem()
		for i := range rv.NumField() {
			field := rv.Field(i)
			newField := newStruct.Field(i)
			if newField.CanSet() {
				newField.Set(deepClone(field))
			}
		}
		return newStruct

	case reflect.Slice:
		if rv.IsNil() {
			return reflect.Zero(rv.Type())
		}
		newSlice := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Cap())
		for i := range rv.Len() {
			newSlice.Index(i).Set(deepClone(rv.Index(i)))
		}
		return newSlice

	case reflect.Map:
		if rv.IsNil() {
			return reflect.Zero(rv.Type())
		}
		newMap := reflect.MakeMap(rv.Type())
		for _, key := range rv.MapKeys() {
			newMap.SetMapIndex(key, deepClone(rv.MapIndex(key)))
		}
		return newMap

	default:
		return rv
	}
}

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...any) {
}

var defaultAuthInternalUsers = []AuthInternalUser{
	{
		User: "any",
		Pass: "",
		Permissions: []AuthInternalUserPermission{
			{
				Action: AuthActionPublish,
			},
			{
				Action: AuthActionRead,
			},
			{
				Action: AuthActionPlayback,
			},
		},
	},
	{
		User: "any",
		Pass: "",
		IPs:  IPNetworks{mustParseCIDR("127.0.0.1/32"), mustParseCIDR("::1/128")},
		Permissions: []AuthInternalUserPermission{
			{
				Action: AuthActionAPI,
			},
			{
				Action: AuthActionMetrics,
			},
			{
				Action: AuthActionPprof,
			},
		},
	},
}

// Conf is a configuration.
type Conf struct {
	// General
	LogLevel            LogLevel        `json:"logLevel"`
	LogDestinations     LogDestinations `json:"logDestinations"`
	LogStructured       bool            `json:"logStructured"`
	LogFile             string          `json:"logFile"`
	SysLogPrefix        string          `json:"sysLogPrefix"`
	ReadTimeout         Duration        `json:"readTimeout"`
	WriteTimeout        Duration        `json:"writeTimeout"`
	ReadBufferCount     *int            `json:"readBufferCount,omitempty"` // deprecated
	WriteQueueSize      int             `json:"writeQueueSize"`
	UDPMaxPayloadSize   int             `json:"udpMaxPayloadSize"`
	UDPReadBufferSize   uint            `json:"udpReadBufferSize"`
	RunOnConnect        string          `json:"runOnConnect"`
	RunOnConnectRestart bool            `json:"runOnConnectRestart"`
	RunOnDisconnect     string          `json:"runOnDisconnect"`

	// Authentication
	AuthMethod                AuthMethod                   `json:"authMethod"`
	AuthInternalUsers         []AuthInternalUser           `json:"authInternalUsers"`
	AuthHTTPAddress           string                       `json:"authHTTPAddress"`
	ExternalAuthenticationURL *string                      `json:"externalAuthenticationURL,omitempty"` // deprecated
	AuthHTTPFingerprint       string                       `json:"authHTTPFingerprint"`
	AuthHTTPExclude           []AuthInternalUserPermission `json:"authHTTPExclude"`
	AuthJWTJWKS               string                       `json:"authJWTJWKS"`
	AuthJWTJWKSFingerprint    string                       `json:"authJWTJWKSFingerprint"`
	AuthJWTClaimKey           string                       `json:"authJWTClaimKey"`
	AuthJWTExclude            []AuthInternalUserPermission `json:"authJWTExclude"`
	AuthJWTInHTTPQuery        bool                         `json:"authJWTInHTTPQuery"`

	// Control API
	API               bool       `json:"api"`
	APIAddress        string     `json:"apiAddress"`
	APIEncryption     bool       `json:"apiEncryption"`
	APIServerKey      string     `json:"apiServerKey"`
	APIServerCert     string     `json:"apiServerCert"`
	APIAllowOrigin    *string    `json:"apiAllowOrigin,omitempty"` // deprecated
	APIAllowOrigins   []string   `json:"apiAllowOrigins"`
	APITrustedProxies IPNetworks `json:"apiTrustedProxies"`

	// Metrics
	Metrics               bool       `json:"metrics"`
	MetricsAddress        string     `json:"metricsAddress"`
	MetricsEncryption     bool       `json:"metricsEncryption"`
	MetricsServerKey      string     `json:"metricsServerKey"`
	MetricsServerCert     string     `json:"metricsServerCert"`
	MetricsAllowOrigin    *string    `json:"metricsAllowOrigin,omitempty"` // deprecated
	MetricsAllowOrigins   []string   `json:"metricsAllowOrigins"`
	MetricsTrustedProxies IPNetworks `json:"metricsTrustedProxies"`

	// PPROF
	PPROF               bool       `json:"pprof"`
	PPROFAddress        string     `json:"pprofAddress"`
	PPROFEncryption     bool       `json:"pprofEncryption"`
	PPROFServerKey      string     `json:"pprofServerKey"`
	PPROFServerCert     string     `json:"pprofServerCert"`
	PPROFAllowOrigin    *string    `json:"pprofAllowOrigin,omitempty"` // deprecated
	PPROFAllowOrigins   []string   `json:"pprofAllowOrigins"`
	PPROFTrustedProxies IPNetworks `json:"pprofTrustedProxies"`

	// Playback
	Playback               bool       `json:"playback"`
	PlaybackAddress        string     `json:"playbackAddress"`
	PlaybackEncryption     bool       `json:"playbackEncryption"`
	PlaybackServerKey      string     `json:"playbackServerKey"`
	PlaybackServerCert     string     `json:"playbackServerCert"`
	PlaybackAllowOrigin    *string    `json:"playbackAllowOrigin,omitempty"` // deprecated
	PlaybackAllowOrigins   []string   `json:"playbackAllowOrigins"`
	PlaybackTrustedProxies IPNetworks `json:"playbackTrustedProxies"`

	// RTSP server
	RTSP                  bool             `json:"rtsp"`
	RTSPDisable           *bool            `json:"rtspDisable,omitempty"` // deprecated
	Protocols             *RTSPTransports  `json:"protocols,omitempty"`   // deprecated
	RTSPTransports        RTSPTransports   `json:"rtspTransports"`
	Encryption            *Encryption      `json:"encryption,omitempty"` // deprecated
	RTSPEncryption        Encryption       `json:"rtspEncryption"`
	RTSPAddress           string           `json:"rtspAddress"`
	RTSPSAddress          string           `json:"rtspsAddress"`
	RTPAddress            string           `json:"rtpAddress"`
	RTCPAddress           string           `json:"rtcpAddress"`
	MulticastIPRange      string           `json:"multicastIPRange"`
	MulticastRTPPort      int              `json:"multicastRTPPort"`
	MulticastRTCPPort     int              `json:"multicastRTCPPort"`
	SRTPAddress           string           `json:"srtpAddress"`
	SRTCPAddress          string           `json:"srtcpAddress"`
	MulticastSRTPPort     int              `json:"multicastSRTPPort"`
	MulticastSRTCPPort    int              `json:"multicastSRTCPPort"`
	ServerKey             *string          `json:"serverKey,omitempty"`
	ServerCert            *string          `json:"serverCert,omitempty"`
	RTSPServerKey         string           `json:"rtspServerKey"`
	RTSPServerCert        string           `json:"rtspServerCert"`
	AuthMethods           *RTSPAuthMethods `json:"authMethods,omitempty"` // deprecated
	RTSPAuthMethods       RTSPAuthMethods  `json:"rtspAuthMethods"`
	RTSPUDPReadBufferSize *uint            `json:"rtspUDPReadBufferSize,omitempty"` // deprecated

	// RTMP server
	RTMP           bool       `json:"rtmp"`
	RTMPDisable    *bool      `json:"rtmpDisable,omitempty"` // deprecated
	RTMPEncryption Encryption `json:"rtmpEncryption"`
	RTMPAddress    string     `json:"rtmpAddress"`
	RTMPSAddress   string     `json:"rtmpsAddress"`
	RTMPServerKey  string     `json:"rtmpServerKey"`
	RTMPServerCert string     `json:"rtmpServerCert"`

	// HLS server
	HLS                bool       `json:"hls"`
	HLSDisable         *bool      `json:"hlsDisable,omitempty"` // deprecated
	HLSAddress         string     `json:"hlsAddress"`
	HLSEncryption      bool       `json:"hlsEncryption"`
	HLSServerKey       string     `json:"hlsServerKey"`
	HLSServerCert      string     `json:"hlsServerCert"`
	HLSAllowOrigin     *string    `json:"hlsAllowOrigin,omitempty"` // deprecated
	HLSAllowOrigins    []string   `json:"hlsAllowOrigins"`
	HLSTrustedProxies  IPNetworks `json:"hlsTrustedProxies"`
	HLSAlwaysRemux     bool       `json:"hlsAlwaysRemux"`
	HLSVariant         HLSVariant `json:"hlsVariant"`
	HLSSegmentCount    int        `json:"hlsSegmentCount"`
	HLSSegmentDuration Duration   `json:"hlsSegmentDuration"`
	HLSPartDuration    Duration   `json:"hlsPartDuration"`
	HLSSegmentMaxSize  StringSize `json:"hlsSegmentMaxSize"`
	HLSDirectory       string     `json:"hlsDirectory"`
	HLSMuxerCloseAfter Duration   `json:"hlsMuxerCloseAfter"`

	// WebRTC server
	WebRTC                      bool              `json:"webrtc"`
	WebRTCDisable               *bool             `json:"webrtcDisable,omitempty"` // deprecated
	WebRTCAddress               string            `json:"webrtcAddress"`
	WebRTCEncryption            bool              `json:"webrtcEncryption"`
	WebRTCServerKey             string            `json:"webrtcServerKey"`
	WebRTCServerCert            string            `json:"webrtcServerCert"`
	WebRTCAllowOrigin           *string           `json:"webrtcAllowOrigin,omitempty"` // deprecated
	WebRTCAllowOrigins          []string          `json:"webrtcAllowOrigins"`
	WebRTCTrustedProxies        IPNetworks        `json:"webrtcTrustedProxies"`
	WebRTCLocalUDPAddress       string            `json:"webrtcLocalUDPAddress"`
	WebRTCLocalTCPAddress       string            `json:"webrtcLocalTCPAddress"`
	WebRTCIPsFromInterfaces     bool              `json:"webrtcIPsFromInterfaces"`
	WebRTCIPsFromInterfacesList []string          `json:"webrtcIPsFromInterfacesList"`
	WebRTCAdditionalHosts       []string          `json:"webrtcAdditionalHosts"`
	WebRTCICEServers2           []WebRTCICEServer `json:"webrtcICEServers2"`
	WebRTCSTUNGatherTimeout     Duration          `json:"webrtcSTUNGatherTimeout"`
	WebRTCHandshakeTimeout      Duration          `json:"webrtcHandshakeTimeout"`
	WebRTCTrackGatherTimeout    Duration          `json:"webrtcTrackGatherTimeout"`
	WebRTCICEUDPMuxAddress      *string           `json:"webrtcICEUDPMuxAddress,omitempty"`  // deprecated
	WebRTCICETCPMuxAddress      *string           `json:"webrtcICETCPMuxAddress,omitempty"`  // deprecated
	WebRTCICEHostNAT1To1IPs     *[]string         `json:"webrtcICEHostNAT1To1IPs,omitempty"` // deprecated
	WebRTCICEServers            *[]string         `json:"webrtcICEServers,omitempty"`        // deprecated

	// SRT server
	SRT        bool   `json:"srt"`
	SRTAddress string `json:"srtAddress"`

	// Record (deprecated)
	Record                *bool         `json:"record,omitempty"`                // deprecated
	RecordPath            *string       `json:"recordPath,omitempty"`            // deprecated
	RecordFormat          *RecordFormat `json:"recordFormat,omitempty"`          // deprecated
	RecordPartDuration    *Duration     `json:"recordPartDuration,omitempty"`    // deprecated
	RecordSegmentDuration *Duration     `json:"recordSegmentDuration,omitempty"` // deprecated
	RecordDeleteAfter     *Duration     `json:"recordDeleteAfter,omitempty"`     // deprecated

	// Path defaults
	PathDefaults Path `json:"pathDefaults"`

	// Paths
	OptionalPaths map[string]*OptionalPath `json:"paths"`
	Paths         map[string]*Path         `json:"-"` // filled by Validate()
}

func (conf *Conf) setDefaults() {
	// General
	conf.LogLevel = LogLevel(logger.Info)
	conf.LogDestinations = LogDestinations{LogDestination(logger.DestinationStdout)}
	conf.LogStructured = false
	conf.LogFile = "mediamtx.log"
	conf.SysLogPrefix = "mediamtx"
	conf.ReadTimeout = 10 * Duration(time.Second)
	conf.WriteTimeout = 10 * Duration(time.Second)
	conf.WriteQueueSize = 512
	conf.UDPMaxPayloadSize = 1452

	// Authentication
	conf.AuthMethod = AuthMethodInternal
	conf.AuthInternalUsers = defaultAuthInternalUsers
	conf.AuthHTTPExclude = []AuthInternalUserPermission{
		{
			Action: AuthActionAPI,
		},
		{
			Action: AuthActionMetrics,
		},
		{
			Action: AuthActionPprof,
		},
	}
	conf.AuthJWTClaimKey = "mediamtx_permissions"
	conf.AuthJWTInHTTPQuery = true

	// Control API
	conf.APIAddress = ":9997"
	conf.APIServerKey = "server.key"
	conf.APIServerCert = "server.crt"
	conf.APIAllowOrigins = []string{"*"}

	// Metrics
	conf.MetricsAddress = ":9998"
	conf.MetricsServerKey = "server.key"
	conf.MetricsServerCert = "server.crt"
	conf.MetricsAllowOrigins = []string{"*"}

	// PPROF
	conf.PPROFAddress = ":9999"
	conf.PPROFServerKey = "server.key"
	conf.PPROFServerCert = "server.crt"
	conf.PPROFAllowOrigins = []string{"*"}

	// Playback server
	conf.PlaybackAddress = ":9996"
	conf.PlaybackServerKey = "server.key"
	conf.PlaybackServerCert = "server.crt"
	conf.PlaybackAllowOrigins = []string{"*"}

	// RTSP server
	conf.RTSP = true
	conf.RTSPEncryption = EncryptionNo
	conf.RTSPTransports = RTSPTransports{
		gortsplib.ProtocolUDP:          {},
		gortsplib.ProtocolUDPMulticast: {},
		gortsplib.ProtocolTCP:          {},
	}
	conf.RTSPAddress = ":8554"
	conf.RTSPSAddress = ":8322"
	conf.RTPAddress = ":8000"
	conf.RTCPAddress = ":8001"
	conf.MulticastIPRange = "224.1.0.0/16"
	conf.MulticastRTPPort = 8002
	conf.MulticastRTCPPort = 8003
	conf.SRTPAddress = ":8004"
	conf.SRTCPAddress = ":8005"
	conf.MulticastSRTPPort = 8006
	conf.MulticastSRTCPPort = 8007
	conf.RTSPServerKey = "server.key"
	conf.RTSPServerCert = "server.crt"
	conf.RTSPAuthMethods = RTSPAuthMethods{RTSPAuthMethod(auth.VerifyMethodBasic)}

	// RTMP server
	conf.RTMP = true
	conf.RTMPEncryption = EncryptionNo
	conf.RTMPAddress = ":1935"
	conf.RTMPSAddress = ":1936"
	conf.RTMPServerKey = "server.key"
	conf.RTMPServerCert = "server.crt"

	// HLS
	conf.HLS = true
	conf.HLSAddress = ":8888"
	conf.HLSServerKey = "server.key"
	conf.HLSServerCert = "server.crt"
	conf.HLSAllowOrigins = []string{"*"}
	conf.HLSVariant = HLSVariant(gohlslib.MuxerVariantLowLatency)
	conf.HLSSegmentCount = 7
	conf.HLSSegmentDuration = 1 * Duration(time.Second)
	conf.HLSPartDuration = 200 * Duration(time.Millisecond)
	conf.HLSSegmentMaxSize = 50 * 1024 * 1024
	conf.HLSMuxerCloseAfter = 60 * Duration(time.Second)

	// WebRTC server
	conf.WebRTC = true
	conf.WebRTCAddress = ":8889"
	conf.WebRTCServerKey = "server.key"
	conf.WebRTCServerCert = "server.crt"
	conf.WebRTCAllowOrigins = []string{"*"}
	conf.WebRTCLocalUDPAddress = ":8189"
	conf.WebRTCIPsFromInterfaces = true
	conf.WebRTCSTUNGatherTimeout = 5 * Duration(time.Second)
	conf.WebRTCHandshakeTimeout = 10 * Duration(time.Second)
	conf.WebRTCTrackGatherTimeout = 2 * Duration(time.Second)

	// SRT server
	conf.SRT = true
	conf.SRTAddress = ":8890"

	conf.PathDefaults.setDefaults()
}

// Load loads a Conf.
func Load(fpath string, defaultConfPaths []string, l logger.Writer) (*Conf, string, error) {
	conf := &Conf{}

	conf.setDefaults()

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

	// disallow nil slices for ease of use and compatibility
	setAllNilSlicesToEmptyRecursive(reflect.ValueOf(conf))

	err = conf.Validate(l)
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

	err = yamlwrapper.Unmarshal(byts, conf)
	if err != nil {
		return "", err
	}

	return fpath, nil
}

// Clone clones the configuration.
func (conf Conf) Clone() *Conf {
	cloned := deepClone(reflect.ValueOf(conf)).Interface().(Conf)
	return &cloned
}

// Validate checks the configuration for errors, converts deprecated fields into new ones, fills dependent fields.
func (conf *Conf) Validate(l logger.Writer) error {
	if l == nil {
		l = &nilLogger{}
	}

	// General (deprecated params)

	if conf.ReadBufferCount != nil {
		l.Log(logger.Warn, "parameter 'readBufferCount' is deprecated and has been replaced with 'writeQueueSize'")
		conf.WriteQueueSize = *conf.ReadBufferCount
	}

	// General

	if conf.ReadTimeout <= 0 {
		return fmt.Errorf("'readTimeout' must be greater than zero")
	}

	if conf.WriteTimeout <= 0 {
		return fmt.Errorf("'writeTimeout' must be greater than zero")
	}

	if (conf.WriteQueueSize & (conf.WriteQueueSize - 1)) != 0 {
		return fmt.Errorf("'writeQueueSize' must be a power of two")
	}

	if conf.UDPMaxPayloadSize > 1472 {
		return fmt.Errorf("'udpMaxPayloadSize' must be less than 1472")
	}

	// Authentication (deprecated params)

	if conf.ExternalAuthenticationURL != nil {
		l.Log(logger.Warn, "parameter 'externalAuthenticationURL' is deprecated "+
			"and has been replaced with 'authMethod' and 'authHTTPAddress'")
		conf.AuthMethod = AuthMethodHTTP
		conf.AuthHTTPAddress = *conf.ExternalAuthenticationURL
	}

	deprecatedCredentialsMode := false
	if anyPathHasDeprecatedCredentials(conf.PathDefaults, conf.OptionalPaths) {
		l.Log(logger.Warn, "you are using one or more authentication-related deprecated parameters "+
			"(publishUser, publishPass, publishIPs, readUser, readPass, readIPs). "+
			"These have been replaced by 'authInternalUsers'")

		if conf.AuthInternalUsers != nil && !reflect.DeepEqual(conf.AuthInternalUsers, defaultAuthInternalUsers) {
			return fmt.Errorf("authInternalUsers and legacy credentials " +
				"(publishUser, publishPass, publishIPs, readUser, readPass, readIPs) cannot be used together")
		}

		conf.AuthInternalUsers = []AuthInternalUser{
			{
				User: "any",
				Permissions: []AuthInternalUserPermission{
					{
						Action: AuthActionPlayback,
					},
				},
			},
			{
				User: "any",
				IPs:  IPNetworks{mustParseCIDR("127.0.0.1/32"), mustParseCIDR("::1/128")},
				Permissions: []AuthInternalUserPermission{
					{
						Action: AuthActionAPI,
					},
					{
						Action: AuthActionMetrics,
					},
					{
						Action: AuthActionPprof,
					},
				},
			},
		}
		deprecatedCredentialsMode = true
	}

	// Authentication

	switch conf.AuthMethod {
	case AuthMethodInternal:
		for _, u := range conf.AuthInternalUsers {
			// https://github.com/bluenviron/gortsplib/blob/55556f1ecfa2bd51b29fe14eddd70512a0361cbd/server_conn.go#L155-L156
			if u.User == "" {
				return fmt.Errorf("empty usernames are not supported")
			}

			if u.User == "any" && u.Pass != "" {
				return fmt.Errorf("using a password with 'any' user is not supported")
			}
		}

	case AuthMethodHTTP:
		if conf.AuthHTTPAddress == "" {
			return fmt.Errorf("'authHTTPAddress' is empty")
		}

		if conf.AuthHTTPAddress != "" &&
			!strings.HasPrefix(conf.AuthHTTPAddress, "http://") &&
			!strings.HasPrefix(conf.AuthHTTPAddress, "https://") {
			return fmt.Errorf("'externalAuthenticationURL' must be a HTTP URL")
		}

	case AuthMethodJWT:
		if conf.AuthJWTJWKS == "" {
			return fmt.Errorf("'authJWTJWKS' is empty")
		}

		if conf.AuthJWTJWKS != "" &&
			!strings.HasPrefix(conf.AuthJWTJWKS, "http://") &&
			!strings.HasPrefix(conf.AuthJWTJWKS, "https://") {
			return fmt.Errorf("'authJWTJWKS' must be a HTTP URL")
		}

		if conf.AuthJWTClaimKey == "" {
			return fmt.Errorf("'authJWTClaimKey' is empty")
		}
	}

	// Control API (deprecated params)

	if conf.APIAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'apiAllowOrigin' is deprecated and has been replaced with 'apiAllowOrigins'")
		conf.APIAllowOrigins = []string{*conf.APIAllowOrigin}
	}

	// Control API

	if conf.API {
		if conf.APIAddress == "" {
			return fmt.Errorf("'apiAddress' must be set when API is enabled")
		}
	}

	// Metrics (deprecated params)

	if conf.MetricsAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'metricsAllowOrigin' is deprecated and has been replaced with 'metricsAllowOrigins'")
		conf.MetricsAllowOrigins = []string{*conf.MetricsAllowOrigin}
	}

	// Metrics

	if conf.Metrics {
		if conf.MetricsAddress == "" {
			return fmt.Errorf("'metricsAddress' must be set when metrics are enabled")
		}
	}

	// PPROF (deprecated params)

	if conf.PPROFAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'pprofAllowOrigin' is deprecated and has been replaced with 'pprofAllowOrigins'")
		conf.PPROFAllowOrigins = []string{*conf.PPROFAllowOrigin}
	}

	// PPROF

	if conf.PPROF {
		if conf.PPROFAddress == "" {
			return fmt.Errorf("'pprofAddress' must be set when pprof is enabled")
		}
	}

	// Playback (deprecated params)

	if conf.PlaybackAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'playbackAllowOrigin' is deprecated and has been replaced with 'playbackAllowOrigins'")
		conf.PlaybackAllowOrigins = []string{*conf.PlaybackAllowOrigin}
	}

	// Playback

	if conf.Playback {
		if conf.PlaybackAddress == "" {
			return fmt.Errorf("'playbackAddress' must be set when playback is enabled")
		}
	}

	// RTSP server (deprecated params)

	if conf.RTSPDisable != nil {
		l.Log(logger.Warn, "parameter 'rtspDisabled' is deprecated and has been replaced with 'rtsp'")
		conf.RTSP = !*conf.RTSPDisable
	}

	if conf.Protocols != nil {
		l.Log(logger.Warn, "parameter 'protocols' is deprecated and has been replaced with 'rtspTransports'")
		conf.RTSPTransports = *conf.Protocols
	}

	if conf.Encryption != nil {
		l.Log(logger.Warn, "parameter 'encryption' is deprecated and has been replaced with 'rtspEncryption'")
		conf.RTSPEncryption = *conf.Encryption
	}

	if conf.AuthMethods != nil {
		l.Log(logger.Warn, "parameter 'authMethods' is deprecated and has been replaced with 'rtspAuthMethods'")
		conf.RTSPAuthMethods = *conf.AuthMethods
	}

	if conf.ServerCert != nil {
		l.Log(logger.Warn, "parameter 'serverCert' is deprecated and has been replaced with 'rtspServerCert'")
		conf.RTSPServerCert = *conf.ServerCert
	}

	if conf.ServerKey != nil {
		l.Log(logger.Warn, "parameter 'serverKey' is deprecated and has been replaced with 'rtspServerKey'")
		conf.RTSPServerKey = *conf.ServerKey
	}

	// RTSP server

	if conf.RTSP {
		if conf.RTSPEncryption == EncryptionNo || conf.RTSPEncryption == EncryptionOptional {
			if conf.RTSPAddress == "" {
				return fmt.Errorf("'rtspAddress' must be set when RTSP is enabled and RTSP encryption is 'no' or 'optional'")
			}

			if _, ok := conf.RTSPTransports[gortsplib.ProtocolUDP]; ok {
				if conf.RTPAddress == "" {
					return fmt.Errorf("'rtpAddress' must be set when UDP is enabled and RTSP encryption is 'no' or 'optional'")
				}
				if conf.RTCPAddress == "" {
					return fmt.Errorf("'rtcpAddress' must be set when UDP is enabled and RTSP encryption is 'no' or 'optional'")
				}
			}

			if _, ok := conf.RTSPTransports[gortsplib.ProtocolUDPMulticast]; ok {
				if conf.MulticastIPRange == "" {
					return fmt.Errorf("'multicastIPRange' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'no' or 'optional'")
				}
				if conf.MulticastRTPPort == 0 {
					return fmt.Errorf("'multicastRTPPort' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'no' or 'optional'")
				}
				if conf.MulticastRTCPPort == 0 {
					return fmt.Errorf("'multicastRTCPPort' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'no' or 'optional'")
				}
			}
		}

		if conf.RTSPEncryption == EncryptionOptional || conf.RTSPEncryption == EncryptionStrict {
			if conf.RTSPSAddress == "" {
				return fmt.Errorf("'rtspsAddress' must be set when RTSP is enabled and RTSP encryption is 'optional' or 'strict'")
			}

			if _, ok := conf.RTSPTransports[gortsplib.ProtocolUDP]; ok {
				if conf.SRTPAddress == "" {
					return fmt.Errorf("'srtpAddress' must be set when UDP is enabled" +
						" and RTSP encryption is 'optional' or 'strict'")
				}
				if conf.SRTCPAddress == "" {
					return fmt.Errorf("'srtcpAddress' must be set when UDP is enabled" +
						" and RTSP encryption is 'optional' or 'strict'")
				}
			}

			if _, ok := conf.RTSPTransports[gortsplib.ProtocolUDPMulticast]; ok {
				if conf.MulticastIPRange == "" {
					return fmt.Errorf("'multicastIPRange' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'optional' or 'strict'")
				}
				if conf.MulticastSRTPPort == 0 {
					return fmt.Errorf("'multicastSRTPPort' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'optional' or 'strict'")
				}
				if conf.MulticastSRTCPPort == 0 {
					return fmt.Errorf("'multicastSRTCPPort' must be set when UDP multicast is enabled" +
						" and RTSP encryption is 'optional' or 'strict'")
				}
			}
		}

		if len(conf.RTSPAuthMethods) == 0 {
			return fmt.Errorf("at least one 'rtspAuthMethods' must be provided")
		}

		if slices.Contains(conf.RTSPAuthMethods, RTSPAuthMethod(auth.VerifyMethodDigestMD5)) {
			if conf.AuthMethod != AuthMethodInternal {
				return fmt.Errorf("when RTSP digest is enabled, the only supported auth method is 'internal'")
			}
			for _, user := range conf.AuthInternalUsers {
				if user.User.IsHashed() || user.Pass.IsHashed() {
					return fmt.Errorf("when RTSP digest is enabled, hashed credentials cannot be used")
				}
			}
		}
	}

	// RTMP (deprecated params)

	if conf.RTMPDisable != nil {
		l.Log(logger.Warn, "parameter 'rtmpDisabled' is deprecated and has been replaced with 'rtmp'")
		conf.RTMP = !*conf.RTMPDisable
	}

	// RTMP

	if conf.RTMP {
		if conf.RTMPAddress == "" {
			return fmt.Errorf("'rtmpAddress' must be set when RTMP is enabled")
		}
	}

	// HLS (deprecated params)

	if conf.HLSDisable != nil {
		l.Log(logger.Warn, "parameter 'hlsDisable' is deprecated and has been replaced with 'hls'")
		conf.HLS = !*conf.HLSDisable
	}

	if conf.HLSAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'hlsAllowOrigin' is deprecated and has been replaced with 'hlsAllowOrigins'")
		conf.HLSAllowOrigins = []string{*conf.HLSAllowOrigin}
	}

	// HLS

	if conf.HLS {
		if conf.HLSAddress == "" {
			return fmt.Errorf("'hlsAddress' must be set when HLS is enabled")
		}
	}

	// WebRTC (deprecated params)

	if conf.WebRTCDisable != nil {
		l.Log(logger.Warn, "parameter 'webrtcDisable' is deprecated and has been replaced with 'webrtc'")
		conf.WebRTC = !*conf.WebRTCDisable
	}

	if conf.WebRTCICEUDPMuxAddress != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEUDPMuxAdderss' is deprecated "+
			"and has been replaced with 'webrtcLocalUDPAddress'")
		conf.WebRTCLocalUDPAddress = *conf.WebRTCICEUDPMuxAddress
	}

	if conf.WebRTCICETCPMuxAddress != nil {
		l.Log(logger.Warn, "parameter 'webrtcICETCPMuxAddress' is deprecated "+
			"and has been replaced with 'webrtcLocalTCPAddress'")
		conf.WebRTCLocalTCPAddress = *conf.WebRTCICETCPMuxAddress
	}

	if conf.WebRTCICEHostNAT1To1IPs != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEHostNAT1To1IPs' is deprecated "+
			"and has been replaced with 'webrtcAdditionalHosts'")
		conf.WebRTCAdditionalHosts = *conf.WebRTCICEHostNAT1To1IPs
	}

	if conf.WebRTCICEServers != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEServers' is deprecated "+
			"and has been replaced with 'webrtcICEServers2'")

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

	if conf.WebRTCAllowOrigin != nil {
		l.Log(logger.Warn, "parameter 'webrtcAllowOrigin' is deprecated and has been replaced with 'webrtcAllowOrigins'")
		conf.WebRTCAllowOrigins = []string{*conf.WebRTCAllowOrigin}
	}

	// WebRTC

	if conf.WebRTC {
		if conf.WebRTCAddress == "" {
			return fmt.Errorf("'webrtcAddress' must be set when WebRTC is enabled")
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
	}

	// Record (deprecated)

	if conf.Record != nil {
		l.Log(logger.Warn, "parameter 'record' is deprecated "+
			"and has been replaced with 'pathDefaults.record'")
		conf.PathDefaults.Record = *conf.Record
	}

	if conf.RecordPath != nil {
		l.Log(logger.Warn, "parameter 'recordPath' is deprecated "+
			"and has been replaced with 'pathDefaults.recordPath'")
		conf.PathDefaults.RecordPath = *conf.RecordPath
	}

	if conf.RecordFormat != nil {
		l.Log(logger.Warn, "parameter 'recordFormat' is deprecated "+
			"and has been replaced with 'pathDefaults.recordFormat'")
		conf.PathDefaults.RecordFormat = *conf.RecordFormat
	}

	if conf.RecordPartDuration != nil {
		l.Log(logger.Warn, "parameter 'recordPartDuration' is deprecated "+
			"and has been replaced with 'pathDefaults.recordPartDuration'")
		conf.PathDefaults.RecordPartDuration = *conf.RecordPartDuration
	}

	if conf.RecordSegmentDuration != nil {
		l.Log(logger.Warn, "parameter 'recordSegmentDuration' is deprecated "+
			"and has been replaced with 'pathDefaults.recordSegmentDuration'")
		conf.PathDefaults.RecordSegmentDuration = *conf.RecordSegmentDuration
	}

	if conf.RecordDeleteAfter != nil {
		l.Log(logger.Warn, "parameter 'recordDeleteAfter' is deprecated "+
			"and has been replaced with 'pathDefaults.recordDeleteAfter'")
		conf.PathDefaults.RecordDeleteAfter = *conf.RecordDeleteAfter
	}

	// paths

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
			conf.OptionalPaths[name] = optional
		}

		pconf := newPath(&conf.PathDefaults, optional)
		conf.Paths[name] = pconf
	}

	for _, name := range sortedKeys(conf.OptionalPaths) {
		err := conf.Paths[name].validate(conf, name, deprecatedCredentialsMode, l)
		if err != nil {
			return err
		}
	}

	return nil
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
	if conf.OptionalPaths == nil {
		conf.OptionalPaths = make(map[string]*OptionalPath)
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
