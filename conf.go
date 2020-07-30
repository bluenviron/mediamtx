package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/aler9/gortsplib"
	"gopkg.in/yaml.v2"
)

type confPath struct {
	Source               string                   `yaml:"source"`
	sourceUrl            *url.URL                 ``
	SourceProtocol       string                   `yaml:"sourceProtocol"`
	sourceProtocolParsed gortsplib.StreamProtocol ``
	SourceOnDemand       bool                     `yaml:"sourceOnDemand"`
	RunOnDemand          string                   `yaml:"runOnDemand"`
	RunOnPublish         string                   `yaml:"runOnPublish"`
	RunOnRead            string                   `yaml:"runOnRead"`
	PublishUser          string                   `yaml:"publishUser"`
	PublishPass          string                   `yaml:"publishPass"`
	PublishIps           []string                 `yaml:"publishIps"`
	publishIpsParsed     []interface{}            ``
	ReadUser             string                   `yaml:"readUser"`
	ReadPass             string                   `yaml:"readPass"`
	ReadIps              []string                 `yaml:"readIps"`
	readIpsParsed        []interface{}            ``
}

type conf struct {
	Protocols             []string                              `yaml:"protocols"`
	protocolsParsed       map[gortsplib.StreamProtocol]struct{} ``
	RtspPort              int                                   `yaml:"rtspPort"`
	RtpPort               int                                   `yaml:"rtpPort"`
	RtcpPort              int                                   `yaml:"rtcpPort"`
	RunOnConnect          string                                `yaml:"runOnConnect"`
	ReadTimeout           time.Duration                         `yaml:"readTimeout"`
	WriteTimeout          time.Duration                         `yaml:"writeTimeout"`
	AuthMethods           []string                              `yaml:"authMethods"`
	authMethodsParsed     []gortsplib.AuthMethod                ``
	Metrics               bool                                  `yaml:"metrics"`
	Pprof                 bool                                  `yaml:"pprof"`
	LogDestinations       []string                              `yaml:"logDestinations"`
	logDestinationsParsed map[logDestination]struct{}           ``
	LogFile               string                                `yaml:"logFile"`
	Paths                 map[string]*confPath                  `yaml:"paths"`
}

func loadConf(fpath string, stdin io.Reader) (*conf, error) {
	conf := &conf{}

	err := func() error {
		if fpath == "stdin" {
			err := yaml.NewDecoder(stdin).Decode(conf)
			if err != nil {
				return err
			}

			return nil

		} else {
			// rtsp-simple-server.yml is optional
			if fpath == "rtsp-simple-server.yml" {
				if _, err := os.Stat(fpath); err != nil {
					return nil
				}
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			err = yaml.NewDecoder(f).Decode(conf)
			if err != nil {
				return err
			}

			return nil
		}
	}()
	if err != nil {
		return nil, err
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = []string{"udp", "tcp"}
	}
	conf.protocolsParsed = make(map[gortsplib.StreamProtocol]struct{})
	for _, proto := range conf.Protocols {
		switch proto {
		case "udp":
			conf.protocolsParsed[gortsplib.StreamProtocolUdp] = struct{}{}

		case "tcp":
			conf.protocolsParsed[gortsplib.StreamProtocolTcp] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(conf.protocolsParsed) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}

	if conf.RtspPort == 0 {
		conf.RtspPort = 8554
	}
	if conf.RtpPort == 0 {
		conf.RtpPort = 8000
	}
	if (conf.RtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if conf.RtcpPort == 0 {
		conf.RtcpPort = 8001
	}
	if conf.RtcpPort != (conf.RtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 5 * time.Second
	}

	if len(conf.AuthMethods) == 0 {
		conf.AuthMethods = []string{"basic", "digest"}
	}
	for _, method := range conf.AuthMethods {
		switch method {
		case "basic":
			conf.authMethodsParsed = append(conf.authMethodsParsed, gortsplib.Basic)

		case "digest":
			conf.authMethodsParsed = append(conf.authMethodsParsed, gortsplib.Digest)

		default:
			return nil, fmt.Errorf("unsupported authentication method: %s", method)
		}
	}

	if len(conf.LogDestinations) == 0 {
		conf.LogDestinations = []string{"stdout"}
	}
	conf.logDestinationsParsed = make(map[logDestination]struct{})
	for _, dest := range conf.LogDestinations {
		switch dest {
		case "stdout":
			conf.logDestinationsParsed[logDestinationStdout] = struct{}{}

		case "file":
			conf.logDestinationsParsed[logDestinationFile] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported log destination: %s", dest)
		}
	}
	if conf.LogFile == "" {
		conf.LogFile = "rtsp-simple-server.log"
	}

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*confPath{
			"all": {},
		}
	}

	for path, confp := range conf.Paths {
		if confp == nil {
			conf.Paths[path] = &confPath{}
			confp = conf.Paths[path]
		}

		if confp.Source == "" {
			confp.Source = "record"
		}

		if confp.Source != "record" {
			if path == "all" {
				return nil, fmt.Errorf("path 'all' cannot have a RTSP source")
			}

			if confp.SourceProtocol == "" {
				confp.SourceProtocol = "udp"
			}

			confp.sourceUrl, err = url.Parse(confp.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid RTSP url", confp.Source)
			}
			if confp.sourceUrl.Scheme != "rtsp" {
				return nil, fmt.Errorf("'%s' is not a valid RTSP url", confp.Source)
			}
			if confp.sourceUrl.Port() == "" {
				confp.sourceUrl.Host += ":554"
			}
			if confp.sourceUrl.User != nil {
				pass, _ := confp.sourceUrl.User.Password()
				user := confp.sourceUrl.User.Username()
				if user != "" && pass == "" ||
					user == "" && pass != "" {
					fmt.Errorf("username and password must be both provided")
				}
			}

			switch confp.SourceProtocol {
			case "udp":
				confp.sourceProtocolParsed = gortsplib.StreamProtocolUdp

			case "tcp":
				confp.sourceProtocolParsed = gortsplib.StreamProtocolTcp

			default:
				return nil, fmt.Errorf("unsupported protocol '%s'", confp.SourceProtocol)
			}
		}

		if confp.PublishUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(confp.PublishUser) {
				return nil, fmt.Errorf("publish username must be alphanumeric")
			}
		}
		if confp.PublishPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(confp.PublishPass) {
				return nil, fmt.Errorf("publish password must be alphanumeric")
			}
		}
		confp.publishIpsParsed, err = parseIpCidrList(confp.PublishIps)
		if err != nil {
			return nil, err
		}

		if confp.ReadUser != "" && confp.ReadPass == "" || confp.ReadUser == "" && confp.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}
		if confp.ReadUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(confp.ReadUser) {
				return nil, fmt.Errorf("read username must be alphanumeric")
			}
		}
		if confp.ReadPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(confp.ReadPass) {
				return nil, fmt.Errorf("read password must be alphanumeric")
			}
		}
		if confp.ReadUser != "" && confp.ReadPass == "" || confp.ReadUser == "" && confp.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}
		confp.readIpsParsed, err = parseIpCidrList(confp.ReadIps)
		if err != nil {
			return nil, err
		}

		if confp.RunOnDemand != "" && path == "all" {
			return nil, fmt.Errorf("option 'runOnDemand' cannot be used in path 'all'")
		}
	}

	return conf, nil
}
