package conf

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/headers"
	"golang.org/x/crypto/nacl/secretbox"
	"gopkg.in/yaml.v2"

	"github.com/aler9/rtsp-simple-server/internal/confenv"
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

// Conf is the main program configuration.
type Conf struct {
	// general
	LogLevel              string                          `yaml:"logLevel"`
	LogLevelParsed        logger.Level                    `yaml:"-" json:"-"`
	LogDestinations       []string                        `yaml:"logDestinations"`
	LogDestinationsParsed map[logger.Destination]struct{} `yaml:"-" json:"-"`
	LogFile               string                          `yaml:"logFile"`
	ListenIP              string                          `yaml:"listenIP"`
	ReadTimeout           time.Duration                   `yaml:"readTimeout"`
	WriteTimeout          time.Duration                   `yaml:"writeTimeout"`
	ReadBufferCount       int                             `yaml:"readBufferCount"`
	Metrics               bool                            `yaml:"metrics"`
	Pprof                 bool                            `yaml:"pprof"`
	RunOnConnect          string                          `yaml:"runOnConnect"`
	RunOnConnectRestart   bool                            `yaml:"runOnConnectRestart"`

	// rtsp
	RTSPDisable       bool                                  `yaml:"rtspDisable"`
	Protocols         []string                              `yaml:"protocols"`
	ProtocolsParsed   map[gortsplib.StreamProtocol]struct{} `yaml:"-" json:"-"`
	Encryption        string                                `yaml:"encryption"`
	EncryptionParsed  Encryption                            `yaml:"-" json:"-"`
	RTSPPort          int                                   `yaml:"rtspPort"`
	RTSPSPort         int                                   `yaml:"rtspsPort"`
	RTPPort           int                                   `yaml:"rtpPort"`
	RTCPPort          int                                   `yaml:"rtcpPort"`
	ServerKey         string                                `yaml:"serverKey"`
	ServerCert        string                                `yaml:"serverCert"`
	AuthMethods       []string                              `yaml:"authMethods"`
	AuthMethodsParsed []headers.AuthMethod                  `yaml:"-" json:"-"`
	ReadBufferSize    int                                   `yaml:"readBufferSize"`

	// rtmp
	RTMPDisable bool `yaml:"rtmpDisable"`
	RTMPPort    int  `yaml:"rtmpPort"`

	// path
	Paths map[string]*PathConf `yaml:"paths"`
}

func (conf *Conf) fillAndCheck() error {
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
		conf.ReadTimeout = 10 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * time.Second
	}
	if conf.ReadBufferCount == 0 {
		conf.ReadBufferCount = 512
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = []string{"udp", "tcp"}
	}
	conf.ProtocolsParsed = make(map[gortsplib.StreamProtocol]struct{})
	for _, proto := range conf.Protocols {
		switch proto {
		case "udp":
			conf.ProtocolsParsed[gortsplib.StreamProtocolUDP] = struct{}{}

		case "tcp":
			conf.ProtocolsParsed[gortsplib.StreamProtocolTCP] = struct{}{}

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

		if _, ok := conf.ProtocolsParsed[gortsplib.StreamProtocolUDP]; ok {
			return fmt.Errorf("encryption can't be used with the UDP stream protocol")
		}

	default:
		return fmt.Errorf("unsupported encryption value: '%s'", conf.Encryption)
	}

	if conf.RTSPPort == 0 {
		conf.RTSPPort = 8554
	}
	if conf.RTSPSPort == 0 {
		conf.RTSPSPort = 8555
	}
	if conf.RTPPort == 0 {
		conf.RTPPort = 8000
	}
	if (conf.RTPPort % 2) != 0 {
		return fmt.Errorf("rtp port must be even")
	}
	if conf.RTCPPort == 0 {
		conf.RTCPPort = 8001
	}
	if conf.RTCPPort != (conf.RTPPort + 1) {
		return fmt.Errorf("rtcp and rtp ports must be consecutive")
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

	if conf.RTMPPort == 0 {
		conf.RTMPPort = 1935
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

		err := pconf.fillAndCheck(name)
		if err != nil {
			return err
		}
	}

	return nil
}

// Load loads a Conf.
func Load(fpath string) (*Conf, bool, error) {
	conf := &Conf{}

	// read from file
	found, err := func() (bool, error) {
		// rtsp-simple-server.yml is optional
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

		err = yaml.Unmarshal(byts, conf)
		if err != nil {
			return true, err
		}

		return true, nil
	}()
	if err != nil {
		return nil, false, err
	}

	// read from environment
	err = confenv.Load("RTSP", conf)
	if err != nil {
		return nil, false, err
	}

	err = conf.fillAndCheck()
	if err != nil {
		return nil, false, err
	}

	return conf, found, nil
}
