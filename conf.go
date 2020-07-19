package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/aler9/gortsplib"
	"gopkg.in/yaml.v2"
)

type ConfPath struct {
	Source           string   `yaml:"source"`
	SourceProtocol   string   `yaml:"sourceProtocol"`
	PublishUser      string   `yaml:"publishUser"`
	PublishPass      string   `yaml:"publishPass"`
	PublishIps       []string `yaml:"publishIps"`
	publishIpsParsed []interface{}
	ReadUser         string   `yaml:"readUser"`
	ReadPass         string   `yaml:"readPass"`
	ReadIps          []string `yaml:"readIps"`
	readIpsParsed    []interface{}
	RunOnPublish     string `yaml:"runOnPublish"`
	RunOnRead        string `yaml:"runOnRead"`
}

type conf struct {
	Protocols         []string `yaml:"protocols"`
	protocolsParsed   map[gortsplib.StreamProtocol]struct{}
	RtspPort          int           `yaml:"rtspPort"`
	RtpPort           int           `yaml:"rtpPort"`
	RtcpPort          int           `yaml:"rtcpPort"`
	RunOnConnect      string        `yaml:"runOnConnect"`
	ReadTimeout       time.Duration `yaml:"readTimeout"`
	WriteTimeout      time.Duration `yaml:"writeTimeout"`
	StreamDeadAfter   time.Duration `yaml:"streamDeadAfter"`
	AuthMethods       []string      `yaml:"authMethods"`
	authMethodsParsed []gortsplib.AuthMethod
	Pprof             bool                 `yaml:"pprof"`
	Paths             map[string]*ConfPath `yaml:"paths"`
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
		conf.ReadTimeout = 5 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 5 * time.Second
	}
	if conf.StreamDeadAfter == 0 {
		conf.StreamDeadAfter = 15 * time.Second
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

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*ConfPath{
			"all": {},
		}
	}

	for path, pconf := range conf.Paths {
		if pconf == nil {
			conf.Paths[path] = &ConfPath{}
			pconf = conf.Paths[path]
		}

		if pconf.Source == "" {
			pconf.Source = "record"
		}

		if pconf.PublishUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishUser) {
				return nil, fmt.Errorf("publish username must be alphanumeric")
			}
		}
		if pconf.PublishPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishPass) {
				return nil, fmt.Errorf("publish password must be alphanumeric")
			}
		}
		pconf.publishIpsParsed, err = parseIpCidrList(pconf.PublishIps)
		if err != nil {
			return nil, err
		}

		if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}
		if pconf.ReadUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadUser) {
				return nil, fmt.Errorf("read username must be alphanumeric")
			}
		}
		if pconf.ReadPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadPass) {
				return nil, fmt.Errorf("read password must be alphanumeric")
			}
		}
		if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}
		pconf.readIpsParsed, err = parseIpCidrList(pconf.ReadIps)
		if err != nil {
			return nil, err
		}

		if pconf.Source != "record" {
			if path == "all" {
				return nil, fmt.Errorf("path 'all' cannot have a RTSP source")
			}

			if pconf.SourceProtocol == "" {
				pconf.SourceProtocol = "udp"
			}
		}
	}

	return conf, nil
}
