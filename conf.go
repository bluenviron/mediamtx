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

type pathConf struct {
	regexp               *regexp.Regexp
	Source               string                   `yaml:"source"`
	sourceUrl            *url.URL                 ``
	SourceProtocol       string                   `yaml:"sourceProtocol"`
	sourceProtocolParsed gortsplib.StreamProtocol ``
	SourceOnDemand       bool                     `yaml:"sourceOnDemand"`
	RunOnInit            string                   `yaml:"runOnInit"`
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
	Paths                 map[string]*pathConf                  `yaml:"paths"`
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
			conf.protocolsParsed[gortsplib.StreamProtocolUDP] = struct{}{}

		case "tcp":
			conf.protocolsParsed[gortsplib.StreamProtocolTCP] = struct{}{}

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

		case "syslog":
			conf.logDestinationsParsed[logDestinationSyslog] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported log destination: %s", dest)
		}
	}
	if conf.LogFile == "" {
		conf.LogFile = "rtsp-simple-server.log"
	}

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*pathConf{
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
			conf.Paths[name] = &pathConf{}
			pconf = conf.Paths[name]
		}

		if name == "" {
			return nil, fmt.Errorf("path name can not be empty")
		}

		// normal path
		if name[0] != '~' {
			err := checkPathName(name)
			if err != nil {
				return nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
			}

			// regular expression path
		} else {
			pathRegexp, err := regexp.Compile(name[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid regular expression: %s", name[1:])
			}
			pconf.regexp = pathRegexp
		}

		if pconf.Source == "" {
			pconf.Source = "record"
		}

		if pconf.Source != "record" {
			if pconf.regexp != nil {
				return nil, fmt.Errorf("a path with a regular expression cannot have a RTSP source; use another path")
			}

			pconf.sourceUrl, err = url.Parse(pconf.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid RTSP url", pconf.Source)
			}
			if pconf.sourceUrl.Scheme != "rtsp" {
				return nil, fmt.Errorf("'%s' is not a valid RTSP url", pconf.Source)
			}
			if pconf.sourceUrl.Port() == "" {
				pconf.sourceUrl.Host += ":554"
			}
			if pconf.sourceUrl.User != nil {
				pass, _ := pconf.sourceUrl.User.Password()
				user := pconf.sourceUrl.User.Username()
				if user != "" && pass == "" ||
					user == "" && pass != "" {
					fmt.Errorf("username and password must be both provided")
				}
			}

			if pconf.SourceProtocol == "" {
				pconf.SourceProtocol = "udp"
			}
			switch pconf.SourceProtocol {
			case "udp":
				pconf.sourceProtocolParsed = gortsplib.StreamProtocolUDP

			case "tcp":
				pconf.sourceProtocolParsed = gortsplib.StreamProtocolTCP

			default:
				return nil, fmt.Errorf("unsupported protocol '%s'", pconf.SourceProtocol)
			}
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

		if pconf.regexp != nil && pconf.RunOnInit != "" {
			return nil, fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
		}
	}

	return conf, nil
}

func (conf *conf) checkPathNameAndFindConf(name string) (*pathConf, error) {
	err := checkPathName(name)
	if err != nil {
		return nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
	}

	// normal path
	if pconf, ok := conf.Paths[name]; ok {
		return pconf, nil
	}

	// regular expression path
	for _, pconf := range conf.Paths {
		if pconf.regexp != nil && pconf.regexp.MatchString(name) {
			return pconf, nil
		}
	}

	return nil, fmt.Errorf("unable to find a valid configuration for path '%s'", name)
}
