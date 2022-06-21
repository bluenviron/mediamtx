package conf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/headers"
	"golang.org/x/crypto/nacl/secretbox"
	"gopkg.in/yaml.v2"

	"github.com/aler9/rtsp-simple-server/internal/logger"
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
	var convert func(i interface{}) (interface{}, error)
	convert = func(i interface{}) (interface{}, error) {
		switch x := i.(type) {
		case map[interface{}]interface{}:
			m2 := map[string]interface{}{}
			for k, v := range x {
				ks, ok := k.(string)
				if !ok {
					return nil, fmt.Errorf("integer keys are not supported (%v)", k)
				}

				m2[ks], err = convert(v)
				if err != nil {
					return nil, err
				}
			}
			return m2, nil

		case []interface{}:
			a2 := make([]interface{}, len(x))
			for i, v := range x {
				a2[i], err = convert(v)
				if err != nil {
					return nil, err
				}
			}
			return a2, nil
		}

		return i, nil
	}
	temp, err = convert(temp)
	if err != nil {
		return false, err
	}

	// check for non-existent parameters
	var checkNonExistentFields func(what interface{}, ref interface{}) error
	checkNonExistentFields = func(what interface{}, ref interface{}) error {
		if what == nil {
			return nil
		}

		ma, ok := what.(map[string]interface{})
		if !ok {
			return fmt.Errorf("not a map")
		}

		for k, v := range ma {
			fi := func() reflect.Type {
				rr := reflect.TypeOf(ref)
				for i := 0; i < rr.NumField(); i++ {
					f := rr.Field(i)
					if f.Tag.Get("json") == k {
						return f.Type
					}
				}
				return nil
			}()
			if fi == nil {
				return fmt.Errorf("non-existent parameter: '%s'", k)
			}

			if fi == reflect.TypeOf(map[string]*PathConf{}) && v != nil {
				ma2, ok := v.(map[string]interface{})
				if !ok {
					return fmt.Errorf("parameter %s is not a map", k)
				}

				for k2, v2 := range ma2 {
					err := checkNonExistentFields(v2, reflect.Zero(fi.Elem().Elem()).Interface())
					if err != nil {
						return fmt.Errorf("parameter %s, key %s: %s", k, k2, err)
					}
				}
			}
		}
		return nil
	}
	err = checkNonExistentFields(temp, Conf{})
	if err != nil {
		return true, err
	}

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
	LogLevel                  LogLevel        `json:"logLevel"`
	LogDestinations           LogDestinations `json:"logDestinations"`
	LogFile                   string          `json:"logFile"`
	ReadTimeout               StringDuration  `json:"readTimeout"`
	WriteTimeout              StringDuration  `json:"writeTimeout"`
	ReadBufferCount           int             `json:"readBufferCount"`
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
	RTSPDisable       bool        `json:"rtspDisable"`
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
	RTMPDisable bool   `json:"rtmpDisable"`
	RTMPAddress string `json:"rtmpAddress"`

	// HLS
	HLSDisable         bool           `json:"hlsDisable"`
	HLSAddress         string         `json:"hlsAddress"`
	HLSAlwaysRemux     bool           `json:"hlsAlwaysRemux"`
	HLSVariant         HLSVariant     `json:"hlsVariant"`
	HLSSegmentCount    int            `json:"hlsSegmentCount"`
	HLSSegmentDuration StringDuration `json:"hlsSegmentDuration"`
	HLSPartDuration    StringDuration `json:"hlsPartDuration"`
	HLSSegmentMaxSize  StringSize     `json:"hlsSegmentMaxSize"`
	HLSAllowOrigin     string         `json:"hlsAllowOrigin"`
	HLSEncryption      bool           `json:"hlsEncryption"`
	HLSServerKey       string         `json:"hlsServerKey"`
	HLSServerCert      string         `json:"hlsServerCert"`
	HLSTrustedProxies  IPsOrCIDRs     `json:"hlsTrustedProxies"`

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

// CheckAndFillMissing checks the configuration for errors and fills missing parameters.
func (conf *Conf) CheckAndFillMissing() error {
	if conf.LogLevel == 0 {
		conf.LogLevel = LogLevel(logger.Info)
	}

	if len(conf.LogDestinations) == 0 {
		conf.LogDestinations = LogDestinations{logger.DestinationStdout: {}}
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

	if conf.ExternalAuthenticationURL != "" {
		if !strings.HasPrefix(conf.ExternalAuthenticationURL, "http://") &&
			!strings.HasPrefix(conf.ExternalAuthenticationURL, "https://") {
			return fmt.Errorf("'externalAuthenticationURL' must be a HTTP URL")
		}
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
		conf.Protocols = Protocols{
			Protocol(gortsplib.TransportUDP):          {},
			Protocol(gortsplib.TransportUDPMulticast): {},
			Protocol(gortsplib.TransportTCP):          {},
		}
	}

	if conf.Encryption == EncryptionStrict {
		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDP)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP transport protocol")
		}

		if _, ok := conf.Protocols[Protocol(gortsplib.TransportUDPMulticast)]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP-multicast transport protocol")
		}
	}

	if conf.RTSPAddress == "" {
		conf.RTSPAddress = ":8554"
	}

	if conf.RTSPSAddress == "" {
		conf.RTSPSAddress = ":8322"
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
		conf.AuthMethods = AuthMethods{headers.AuthBasic, headers.AuthDigest}
	}

	if conf.RTMPAddress == "" {
		conf.RTMPAddress = ":1935"
	}

	if conf.HLSAddress == "" {
		conf.HLSAddress = ":8888"
	}

	if conf.HLSSegmentCount == 0 {
		conf.HLSSegmentCount = 7
	}

	if conf.HLSSegmentDuration == 0 {
		conf.HLSSegmentDuration = 1 * StringDuration(time.Second)
	}

	if conf.HLSPartDuration == 0 {
		conf.HLSPartDuration = 200 * StringDuration(time.Millisecond)
	}

	if conf.HLSSegmentMaxSize == 0 {
		conf.HLSSegmentMaxSize = 50 * 1024 * 1024
	}

	if conf.HLSAllowOrigin == "" {
		conf.HLSAllowOrigin = "*"
	}

	if conf.HLSServerKey == "" {
		conf.HLSServerKey = "server.key"
	}

	if conf.HLSServerCert == "" {
		conf.HLSServerCert = "server.crt"
	}

	switch conf.HLSVariant {
	case HLSVariantLowLatency:
		if conf.HLSSegmentCount < 7 {
			return fmt.Errorf("Low-Latency HLS requires at least 7 segments")
		}

		if !conf.HLSEncryption {
			return fmt.Errorf("Low-Latency HLS requires encryption")
		}

	default:
		if conf.HLSSegmentCount < 3 {
			return fmt.Errorf("The minimum number of HLS segments is 3")
		}
	}

	// do not add automatically "all", since user may want to
	// initialize all paths through API or hot reloading.
	if conf.Paths == nil {
		conf.Paths = make(map[string]*PathConf)
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

		err := pconf.checkAndFillMissing(conf, name)
		if err != nil {
			return err
		}
	}

	return nil
}
