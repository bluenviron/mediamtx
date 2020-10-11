package main

import (
	"fmt"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/spf13/pflag"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/headers"
)

const (
	envPrefix        = "RTSP_"
	keyPathDelimiter = "."
)

type pathConf struct {
	regexp               *regexp.Regexp
	Source               string                   `flag:"source"`
	sourceUrl            *url.URL                 `flag:"-"`
	SourceProtocol       string                   `flag:"sourceProtocol"`
	sourceProtocolParsed gortsplib.StreamProtocol `flag:"-"`
	SourceOnDemand       bool                     `flag:"sourceOnDemand"`
	RunOnInit            string                   `flag:"runOnInit"`
	RunOnDemand          string                   `flag:"runOnDemand"`
	RunOnPublish         string                   `flag:"runOnPublish"`
	RunOnRead            string                   `flag:"runOnRead"`
	PublishUser          string                   `flag:"publishUser"`
	PublishPass          string                   `flag:"publishPass"`
	PublishIps           []string                 `flag:"publishIps"`
	publishIpsParsed     []interface{}            `flag:"-"`
	ReadUser             string                   `flag:"readUser"`
	ReadPass             string                   `flag:"readPass"`
	ReadIps              []string                 `flag:"readIps"`
	readIpsParsed        []interface{}            `flag:"-"`
}

type conf struct {
	Version               bool                                  `flag:"version,v;;show version"`
	Protocols             []string                              `flag:"protocols;;supported stream protocols (the handshake is always performed with TCP)"`
	protocolsParsed       map[gortsplib.StreamProtocol]struct{} `flag:"-"`
	RtspPort              int                                   `flag:"rtspPort;;port of the TCP RTSP listener"`
	RtpPort               int                                   `flag:"rtpPort;;port of the UDP RTP listener (used only if udp is in protocols)"`
	RtcpPort              int                                   `flag:"rtcpPort;;port of the UDP RTCP listener (used only if udp is in protocols)"`
	RunOnConnect          string                                `flag:"runOnConnect;;command to run when a client connects, this is terminated with SIGINT when a client disconnects."`
	ReadTimeout           time.Duration                         `flag:"readTimeout;;timeout of read operations"`
	WriteTimeout          time.Duration                         `flag:"writeTimeout;;timeout of write operations"`
	AuthMethods           []string                              `flag:"authMethods;;supported authentication methods (both are insecure, use RTSP inside a VPN to enforce security)"`
	authMethodsParsed     []headers.AuthMethod                  `flag:"-"`
	Metrics               bool                                  `flag:"metrics;;enable Prometheus-compatible metrics on port 9998"`
	Pprof                 bool                                  `flag:"pprof;;enable pprof on port 9999 to monitor performances"`
	LogDestinations       []string                              `flag:"logDestinations;;destinations of log messages, available options are 'stdout', 'file' and 'syslog'"`
	logDestinationsParsed map[logDestination]struct{}           `flag:"-"`
	LogFile               string                                `flag:"logFile;;if 'file' is in logDestinations, this is the file that will receive the logs"`
	Paths                 map[string]*pathConf                  `flag:"paths"`
}

type confLoader struct {
	k   *koanf.Koanf
	err error
}

func (cl *confLoader) loadDefaultValue() *confLoader {
	if cl.err != nil {
		return cl
	}
	return cl.load(confmap.Provider(map[string]interface{}{
		"protocols":       []string{"udp", "tcp"},
		"rtspPort":        8554,
		"rtpPort":         8000,
		"rtcpPort":        8001,
		"readTimeout":     10 * time.Second,
		"writeTimeout":    5 * time.Second,
		"authMethods":     []string{"basic", "digest"},
		"logDestinations": []string{"stdout"},
		"logFile":         "rtsp-simple-server.log",
	}, ""), nil)
}

func (cl *confLoader) loadFromArg(fpath string, stdin io.Reader) *confLoader {
	if cl.err != nil {
		return cl
	}
	var p koanf.Provider
	if fpath == "stdin" {
		b, err := ioutil.ReadAll(stdin)
		if err != nil {
			cl.err = err
			return cl
		}
		p = rawbytes.Provider(b)
	} else {
		// rtsp-simple-server.yml is optional
		if fpath == "rtsp-simple-server.yml" {
			if _, err := os.Stat(fpath); err != nil {
				return cl
			}
		}
		p = file.Provider(fpath)
	}
	return cl.load(p, yaml.Parser())
}

func (cl *confLoader) loadFromFlags(fs *pflag.FlagSet) *confLoader {
	if cl.err != nil {
		return cl
	}
	return cl.load(posflag.Provider(fs, keyPathDelimiter, cl.k), nil)
}

func (cl *confLoader) loadFromEnv() *confLoader {
	if cl.err != nil {
		return cl
	}
	return cl.load(env.Provider(envPrefix, keyPathDelimiter, func(s string) string {
		return strings.Replace(strings.TrimPrefix(s, envPrefix), "_", keyPathDelimiter, -1)
	}), nil)
}

func (cl *confLoader) load(p koanf.Provider, pa koanf.Parser) *confLoader {
	if cl.err == nil {
		cl.err = cl.k.Load(p, pa)
	}
	return cl
}

func (cl confLoader) toConf() (*conf, error) {
	if cl.err != nil {
		return nil, cl.err
	}
	var c conf
	if err := cl.k.Unmarshal("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func loadConf(fpath string, stdin io.Reader, fs *pflag.FlagSet) (*conf, error) {
	cl := confLoader{k: koanf.New(keyPathDelimiter)}
	// load config order: default -> file / stdin -> flags -> env
	c, err := cl.
		loadDefaultValue().
		loadFromArg(fpath, stdin).
		loadFromFlags(fs).
		loadFromEnv().
		toConf()
	if err != nil {
		return nil, err
	}

	c.protocolsParsed = make(map[gortsplib.StreamProtocol]struct{})
	for _, proto := range c.Protocols {
		switch proto {
		case "udp":
			c.protocolsParsed[gortsplib.StreamProtocolUDP] = struct{}{}

		case "tcp":
			c.protocolsParsed[gortsplib.StreamProtocolTCP] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(c.protocolsParsed) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}

	if (c.RtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if c.RtcpPort != (c.RtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}

	for _, method := range c.AuthMethods {
		switch method {
		case "basic":
			c.authMethodsParsed = append(c.authMethodsParsed, headers.AuthBasic)

		case "digest":
			c.authMethodsParsed = append(c.authMethodsParsed, headers.AuthDigest)

		default:
			return nil, fmt.Errorf("unsupported authentication method: %s", method)
		}
	}

	c.logDestinationsParsed = make(map[logDestination]struct{})
	for _, dest := range c.LogDestinations {
		switch dest {
		case "stdout":
			c.logDestinationsParsed[logDestinationStdout] = struct{}{}

		case "file":
			c.logDestinationsParsed[logDestinationFile] = struct{}{}

		case "syslog":
			c.logDestinationsParsed[logDestinationSyslog] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported log destination: %s", dest)
		}
	}

	if len(c.Paths) == 0 {
		c.Paths = map[string]*pathConf{
			"all": {},
		}
	}

	// "all" is an alias for "~^.*$"
	if _, ok := c.Paths["all"]; ok {
		c.Paths["~^.*$"] = c.Paths["all"]
		delete(c.Paths, "all")
	}

	for name, pconf := range c.Paths {
		if pconf == nil {
			c.Paths[name] = &pathConf{}
			pconf = c.Paths[name]
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

		if strings.HasPrefix(pconf.Source, "rtsp://") {
			if pconf.regexp != nil {
				return nil, fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source; use another path")
			}

			pconf.sourceUrl, err = url.Parse(pconf.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid url", pconf.Source)
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

		} else if strings.HasPrefix(pconf.Source, "rtmp://") {
			if pconf.regexp != nil {
				return nil, fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source; use another path")
			}

			pconf.sourceUrl, err = url.Parse(pconf.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid url", pconf.Source)
			}
			if pconf.sourceUrl.Port() == "" {
				pconf.sourceUrl.Host += ":1935"
			}

		} else if pconf.Source == "record" {

		} else {
			return nil, fmt.Errorf("unsupported source: '%s'", pconf.Source)
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

	return c, nil
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
