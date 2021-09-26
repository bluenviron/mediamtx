package conf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aler9/gortsplib/pkg/headers"
	"golang.org/x/crypto/nacl/secretbox"
	"gopkg.in/yaml.v2"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// Encryption is an encryption policy.
type Encryption int

// encryption policies.
const (
	EncryptionNo Encryption = iota
	EncryptionOptional
	EncryptionStrict
)

// Protocol is a RTSP protocol
type Protocol int

// RTSP protocols.
const (
	ProtocolUDP Protocol = iota
	ProtocolMulticast
	ProtocolTCP
)

func decrypt(key string, byts []byte) ([]byte, error) {
	enc, err := base64.StdEncoding.DecodeString(string(byts))
	if err != nil {
		return nil, err
	}

	var secretKey [32]byte
	copy(secretKey[:], key)

	var decryptNonce [24]byte
	copy(decryptNonce[:], enc[:24])
	decrypted, ok := secretbox.Open(nil, enc[24:], &decryptNonce, &secretKey)
	if !ok {
		return nil, fmt.Errorf("decryption error")
	}

	return decrypted, nil
}

func loadFromFile(fpath string, conf *Conf) (bool, error) {
	// rtsp-simple-server.yml is optional
	// other configuration files are not
	if fpath == "rtsp-simple-server.yml" {
		if _, err := os.Stat(fpath); err != nil {
			return false, nil
		}
	}

	byts, err := ioutil.ReadFile(fpath)
	if err != nil {
		return true, err
	}

	if key, ok := os.LookupEnv("RTSP_CONFKEY"); ok {
		byts, err = decrypt(key, byts)
		if err != nil {
			return true, err
		}
	}

	// load YAML config into a generic map
	var temp interface{}
	err = yaml.Unmarshal(byts, &temp)
	if err != nil {
		return true, err
	}

	// convert interface{} keys into string keys to avoid JSON errors
	var convert func(i interface{}) interface{}
	convert = func(i interface{}) interface{} {
		switch x := i.(type) {
		case map[interface{}]interface{}:
			m2 := map[string]interface{}{}
			for k, v := range x {
				m2[k.(string)] = convert(v)
			}
			return m2
		case []interface{}:
			a2 := make([]interface{}, len(x))
			for i, v := range x {
				a2[i] = convert(v)
			}
			return a2
		}
		return i
	}
	temp = convert(temp)

	// convert the generic map into JSON
	byts, err = json.Marshal(temp)
	if err != nil {
		return true, err
	}

	// load the configuration from JSON
	err = json.Unmarshal(byts, conf)
	if err != nil {
		return true, err
	}

	return true, nil
}

// Conf is a configuration.
type Conf struct {
	// general
	LogLevel              string                          `json:"logLevel"`
	LogLevelParsed        logger.Level                    `json:"-"`
	LogDestinations       []string                        `json:"logDestinations"`
	LogDestinationsParsed map[logger.Destination]struct{} `json:"-"`
	LogFile               string                          `json:"logFile"`
	ReadTimeout           StringDuration                  `json:"readTimeout"`
	WriteTimeout          StringDuration                  `json:"writeTimeout"`
	ReadBufferCount       int                             `json:"readBufferCount"`
	API                   bool                            `json:"api"`
	APIAddress            string                          `json:"apiAddress"`
	Metrics               bool                            `json:"metrics"`
	MetricsAddress        string                          `json:"metricsAddress"`
	PPROF                 bool                            `json:"pprof"`
	PPROFAddress          string                          `json:"pprofAddress"`
	RunOnConnect          string                          `json:"runOnConnect"`
	RunOnConnectRestart   bool                            `json:"runOnConnectRestart"`

	// RTSP
	RTSPDisable       bool                  `json:"rtspDisable"`
	Protocols         []string              `json:"protocols"`
	ProtocolsParsed   map[Protocol]struct{} `json:"-"`
	Encryption        string                `json:"encryption"`
	EncryptionParsed  Encryption            `json:"-"`
	RTSPAddress       string                `json:"rtspAddress"`
	RTSPSAddress      string                `json:"rtspsAddress"`
	RTPAddress        string                `json:"rtpAddress"`
	RTCPAddress       string                `json:"rtcpAddress"`
	MulticastIPRange  string                `json:"multicastIPRange"`
	MulticastRTPPort  int                   `json:"multicastRTPPort"`
	MulticastRTCPPort int                   `json:"multicastRTCPPort"`
	ServerKey         string                `json:"serverKey"`
	ServerCert        string                `json:"serverCert"`
	AuthMethods       []string              `json:"authMethods"`
	AuthMethodsParsed []headers.AuthMethod  `json:"-"`
	ReadBufferSize    int                   `json:"readBufferSize"`

	// RTMP
	RTMPDisable bool   `json:"rtmpDisable"`
	RTMPAddress string `json:"rtmpAddress"`

	// HLS
	HLSDisable         bool           `json:"hlsDisable"`
	HLSAddress         string         `json:"hlsAddress"`
	HLSAlwaysRemux     bool           `json:"hlsAlwaysRemux"`
	HLSSegmentCount    int            `json:"hlsSegmentCount"`
	HLSSegmentDuration StringDuration `json:"hlsSegmentDuration"`
	HLSAllowOrigin     string         `json:"hlsAllowOrigin"`

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

	err = loadFromEnvironment("RTSP", conf)
	if err != nil {
		return nil, false, err
	}

	err = conf.CheckAndFillMissing()
	if err != nil {
		return nil, false, err
	}

	return conf, found, nil
}

// CheckAndFillMissing checks the configuration for errors and fills missing fields.
func (conf *Conf) CheckAndFillMissing() error {
	if conf.LogLevel == "" {
		conf.LogLevel = "info"
	}
	switch conf.LogLevel {
	case "warn":
		conf.LogLevelParsed = logger.Warn

	case "info":
		conf.LogLevelParsed = logger.Info

	case "debug":
		conf.LogLevelParsed = logger.Debug

	default:
		return fmt.Errorf("unsupported log level: %s", conf.LogLevel)
	}

	if len(conf.LogDestinations) == 0 {
		conf.LogDestinations = []string{"stdout"}
	}
	conf.LogDestinationsParsed = make(map[logger.Destination]struct{})
	for _, dest := range conf.LogDestinations {
		switch dest {
		case "stdout":
			conf.LogDestinationsParsed[logger.DestinationStdout] = struct{}{}

		case "file":
			conf.LogDestinationsParsed[logger.DestinationFile] = struct{}{}

		case "syslog":
			conf.LogDestinationsParsed[logger.DestinationSyslog] = struct{}{}

		default:
			return fmt.Errorf("unsupported log destination: %s", dest)
		}
	}

	if conf.LogFile == "" {
		conf.LogFile = "rtsp-simple-server.log"
	}
	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 * StringDuration(time.Second)
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * StringDuration(time.Second)
	}
	if conf.ReadBufferCount == 0 {
		conf.ReadBufferCount = 512
	}

	if conf.APIAddress == "" {
		conf.APIAddress = "127.0.0.1:9997"
	}

	if conf.MetricsAddress == "" {
		conf.MetricsAddress = "127.0.0.1:9998"
	}

	if conf.PPROFAddress == "" {
		conf.PPROFAddress = "127.0.0.1:9999"
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = []string{"udp", "multicast", "tcp"}
	}
	conf.ProtocolsParsed = make(map[Protocol]struct{})
	for _, proto := range conf.Protocols {
		switch proto {
		case "udp":
			conf.ProtocolsParsed[ProtocolUDP] = struct{}{}

		case "multicast":
			conf.ProtocolsParsed[ProtocolMulticast] = struct{}{}

		case "tcp":
			conf.ProtocolsParsed[ProtocolTCP] = struct{}{}

		default:
			return fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(conf.ProtocolsParsed) == 0 {
		return fmt.Errorf("no protocols provided")
	}

	if conf.Encryption == "" {
		conf.Encryption = "no"
	}
	switch conf.Encryption {
	case "no", "false":
		conf.EncryptionParsed = EncryptionNo

	case "optional":
		conf.EncryptionParsed = EncryptionOptional

	case "strict", "yes", "true":
		conf.EncryptionParsed = EncryptionStrict

		if _, ok := conf.ProtocolsParsed[ProtocolUDP]; ok {
			return fmt.Errorf("encryption can't be used with the UDP stream protocol")
		}

	default:
		return fmt.Errorf("unsupported encryption value: '%s'", conf.Encryption)
	}

	if conf.RTSPAddress == "" {
		conf.RTSPAddress = ":8554"
	}
	if conf.RTSPSAddress == "" {
		conf.RTSPSAddress = ":8555"
	}
	if conf.RTPAddress == "" {
		conf.RTPAddress = ":8000"
	}
	if conf.RTCPAddress == "" {
		conf.RTCPAddress = ":8001"
	}
	if conf.MulticastIPRange == "" {
		conf.MulticastIPRange = "224.1.0.0/16"
	}
	if conf.MulticastRTPPort == 0 {
		conf.MulticastRTPPort = 8002
	}
	if conf.MulticastRTCPPort == 0 {
		conf.MulticastRTCPPort = 8003
	}

	if conf.ServerKey == "" {
		conf.ServerKey = "server.key"
	}
	if conf.ServerCert == "" {
		conf.ServerCert = "server.crt"
	}

	if len(conf.AuthMethods) == 0 {
		conf.AuthMethods = []string{"basic", "digest"}
	}
	for _, method := range conf.AuthMethods {
		switch method {
		case "basic":
			conf.AuthMethodsParsed = append(conf.AuthMethodsParsed, headers.AuthBasic)

		case "digest":
			conf.AuthMethodsParsed = append(conf.AuthMethodsParsed, headers.AuthDigest)

		default:
			return fmt.Errorf("unsupported authentication method: %s", method)
		}
	}

	if conf.RTMPAddress == "" {
		conf.RTMPAddress = ":1935"
	}

	if conf.HLSAddress == "" {
		conf.HLSAddress = ":8888"
	}
	if conf.HLSSegmentCount == 0 {
		conf.HLSSegmentCount = 3
	}
	if conf.HLSSegmentDuration == 0 {
		conf.HLSSegmentDuration = 1 * StringDuration(time.Second)
	}
	if conf.HLSAllowOrigin == "" {
		conf.HLSAllowOrigin = "*"
	}

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*PathConf{
			"all": {},
		}
	}

	// "all" is an alias for "~^.*$"
	if _, ok := conf.Paths["all"]; ok {
		conf.Paths["~^.*$"] = conf.Paths["all"]
		delete(conf.Paths, "all")
	}

	for name, pconf := range conf.Paths {
		if pconf == nil {
			conf.Paths[name] = &PathConf{}
			pconf = conf.Paths[name]
		}

		err := pconf.checkAndFillMissing(name)
		if err != nil {
			return err
		}
	}

	return nil
}
