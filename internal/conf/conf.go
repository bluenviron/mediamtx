package conf

import (
	"fmt"
	"os"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/headers"
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

// Conf is the main program configuration.
type Conf struct {
	LogLevel              string                                `yaml:"logLevel"`
	LogLevelParsed        logger.Level                          `yaml:"-" json:"-"`
	LogDestinations       []string                              `yaml:"logDestinations"`
	LogDestinationsParsed map[logger.Destination]struct{}       `yaml:"-" json:"-"`
	LogFile               string                                `yaml:"logFile"`
	Protocols             []string                              `yaml:"protocols"`
	ProtocolsParsed       map[gortsplib.StreamProtocol]struct{} `yaml:"-" json:"-"`
	Encryption            string                                `yaml:"encryption"`
	EncryptionParsed      Encryption                            `yaml:"-" json:"-"`
	RtspPort              int                                   `yaml:"rtspPort"`
	RtspsPort             int                                   `yaml:"rtspsPort"`
	RtpPort               int                                   `yaml:"rtpPort"`
	RtcpPort              int                                   `yaml:"rtcpPort"`
	ServerKey             string                                `yaml:"serverKey"`
	ServerCert            string                                `yaml:"serverCert"`
	AuthMethods           []string                              `yaml:"authMethods"`
	AuthMethodsParsed     []headers.AuthMethod                  `yaml:"-" json:"-"`
	ReadTimeout           time.Duration                         `yaml:"readTimeout"`
	WriteTimeout          time.Duration                         `yaml:"writeTimeout"`
	Metrics               bool                                  `yaml:"metrics"`
	Pprof                 bool                                  `yaml:"pprof"`
	RunOnConnect          string                                `yaml:"runOnConnect"`
	RunOnConnectRestart   bool                                  `yaml:"runOnConnectRestart"`
	Paths                 map[string]*PathConf                  `yaml:"paths"`
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

	if conf.RtspPort == 0 {
		conf.RtspPort = 8554
	}
	if conf.RtspsPort == 0 {
		conf.RtspsPort = 8555
	}
	if conf.RtpPort == 0 {
		conf.RtpPort = 8000
	}
	if (conf.RtpPort % 2) != 0 {
		return fmt.Errorf("rtp port must be even")
	}
	if conf.RtcpPort == 0 {
		conf.RtcpPort = 8001
	}
	if conf.RtcpPort != (conf.RtpPort + 1) {
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

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * time.Second
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

		f, err := os.Open(fpath)
		if err != nil {
			return false, err
		}
		defer f.Close()

		err = yaml.NewDecoder(f).Decode(conf)
		if err != nil {
			return false, err
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
