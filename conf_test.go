package main

import (
	"bytes"
	"github.com/AdamSLevy/flagbind"
	"github.com/knadh/koanf"
	"github.com/spf13/pflag"
	"os"
	"testing"
	"time"
)

func TestConfLoader_loadDefaultValues(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}

	actualConf, err := cl.loadDefaultValue().toConf()
	expectedConf := conf{
		Protocols:       []string{"udp", "tcp"},
		RtspPort:        8554,
		RtpPort:         8000,
		RtcpPort:        8001,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		AuthMethods:     []string{"basic", "digest"},
		LogDestinations: []string{"stdout"},
		LogFile:         "rtsp-simple-server.log",
	}
	check(t, err, expectedConf, actualConf)
}

func TestConfLoader_loadFromArg(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}

	actualConf, err := cl.loadFromArg("rtsp-simple-server.yml", nil).toConf()

	expectedConf := conf{
		Protocols:       []string{"udp", "tcp"},
		RtspPort:        8554,
		RtpPort:         8000,
		RtcpPort:        8001,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		AuthMethods:     []string{"basic", "digest"},
		LogDestinations: []string{"stdout"},
		LogFile:         "rtsp-simple-server.log",
	}
	check(t, err, expectedConf, actualConf)
}

func TestConfLoader_loadFromStdin(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}
	buffer := bytes.NewBufferString(`
protocols: [udp, tcp]
rtspPort: 8554
rtpPort: 8000
rtcpPort: 8001
readTimeout: 10s
writeTimeout: 5s
authMethods: [basic, digest]
runOnConnect:
metrics: no
pprof: no
logDestinations: [stdout]
logFile: rtsp-simple-server.log
`)
	actualConf, err := cl.loadFromArg("stdin", buffer).toConf()

	expectedConf := conf{
		Protocols:       []string{"udp", "tcp"},
		RtspPort:        8554,
		RtpPort:         8000,
		RtcpPort:        8001,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		AuthMethods:     []string{"basic", "digest"},
		LogDestinations: []string{"stdout"},
		LogFile:         "rtsp-simple-server.log",
	}
	check(t, err, expectedConf, actualConf)
}

func TestConfLoader_loadFromFlags(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}
	fs := pflag.NewFlagSet("rtsp-simple-server", pflag.ContinueOnError)
	if err := flagbind.Bind(fs, &conf{}); err != nil {
		t.Fatalf("Could not bind the flags: %v", err)
	}
	args := []string{
		"--protocols", "udp,tcp",
		"--rtspPort", "8554",
		"--rtpPort", "8000",
		"--rtcpPort", "8001",
		"--readTimeout", "10s",
		"--writeTimeout", "5s",
		"--authMethods", "basic,digest",
		"--logDestinations", "stdout",
		"--logFile", "rtsp-simple-server.log",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Could not parse flags: %v", err)
	}

	actualConf, err := cl.loadFromFlags(fs).toConf()

	expectedConf := conf{
		Protocols:       []string{"udp", "tcp"},
		RtspPort:        8554,
		RtpPort:         8000,
		RtcpPort:        8001,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		AuthMethods:     []string{"basic", "digest"},
		LogDestinations: []string{"stdout"},
		LogFile:         "rtsp-simple-server.log",
		Paths: map[string]*pathConf{
			"all": {
				Source: "record",
			},
		},
	}
	check(t, err, expectedConf, actualConf)
}

func TestConfLoader_loadFromEnv(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}
	// for now, koanf cannot provide string slice from environment variables unfortunately
	// see https://github.com/knadh/koanf/issues/33
	os.Setenv(envPrefix+"protocols", "udp")
	os.Setenv(envPrefix+"rtspPort", "8554")
	os.Setenv(envPrefix+"rtpPort", "8000")
	os.Setenv(envPrefix+"rtcpPort", "8001")
	os.Setenv(envPrefix+"readTimeout", "10s")
	os.Setenv(envPrefix+"writeTimeout", "5s")
	os.Setenv(envPrefix+"authMethods", "basic")
	os.Setenv(envPrefix+"logDestinations", "stdout")
	os.Setenv(envPrefix+"logFile", "rtsp-simple-server.log")
	defer func() {
		os.Unsetenv(envPrefix+"protocols")
		os.Unsetenv(envPrefix+"rtspPort")
		os.Unsetenv(envPrefix+"rtpPort")
		os.Unsetenv(envPrefix+"rtcpPort")
		os.Unsetenv(envPrefix+"readTimeout")
		os.Unsetenv(envPrefix+"writeTimeout")
		os.Unsetenv(envPrefix+"authMethods")
		os.Unsetenv(envPrefix+"logDestinations")
		os.Unsetenv(envPrefix+"logFile")
	}()

	actualConf, err := cl.loadFromEnv().toConf()

	expectedConf := conf{
		Protocols:       []string{"udp"},
		RtspPort:        8554,
		RtpPort:         8000,
		RtcpPort:        8001,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		AuthMethods:     []string{"basic"},
		LogDestinations: []string{"stdout"},
		LogFile:         "rtsp-simple-server.log",
	}
	check(t, err, expectedConf, actualConf)
}

func TestConfLoader_loadFromAll(t *testing.T) {
	cl := confLoader{k: koanf.New(".")}
	buffer := bytes.NewBufferString("rtspPort: 8555")
	fs := pflag.NewFlagSet("rtsp-simple-server", pflag.ContinueOnError)
	flagbind.Bind(fs, &conf{})
	fs.Parse([]string{"--rtspPort", "8556"})
	os.Setenv(envPrefix+"rtspPort", "8557")
	defer func() {os.Unsetenv(envPrefix+"rtspPort")}()

	actualConf, err := cl.
		loadDefaultValue().
		loadFromArg("stdin", buffer).
		loadFromFlags(fs).
		loadFromEnv().
		toConf()

	if err != nil {
		t.Fatalf("There should be no error. Actual error: %s", err)
	}
	expectedRtspPort := 8557
	if expectedRtspPort != actualConf.RtspPort {
		t.Errorf("expected %v, actual %v", expectedRtspPort, actualConf.RtspPort)
	}
}

func check(t *testing.T, err error, expectedConf conf, actualConf *conf) {
	if err != nil {
		t.Fatalf("There should be no error. Actual error: %s", err)
	}
	if expectedConf.RtspPort != actualConf.RtspPort {
		t.Errorf("expected %v, actual %v", expectedConf.RtspPort, actualConf.RtspPort)
	}
	if expectedConf.RtpPort != actualConf.RtpPort {
		t.Errorf("expected %v, actual %v", expectedConf.RtpPort, actualConf.RtpPort)
	}
	if expectedConf.RtcpPort != actualConf.RtcpPort {
		t.Errorf("expected %v, actual %v", expectedConf.RtcpPort, actualConf.RtcpPort)
	}
	if expectedConf.ReadTimeout != actualConf.ReadTimeout {
		t.Errorf("expected %v, actual %v", expectedConf.ReadTimeout, actualConf.ReadTimeout)
	}
	if expectedConf.WriteTimeout != actualConf.WriteTimeout {
		t.Errorf("expected %v, actual %v", expectedConf.WriteTimeout, actualConf.WriteTimeout)
	}
	if expectedConf.LogFile != actualConf.LogFile {
		t.Errorf("expected %v, actual %v", expectedConf.LogFile, actualConf.LogFile)
	}
	if len(expectedConf.Protocols) != len(actualConf.Protocols) {
		t.Fatalf("expected protocols length %d, actual protocols length %d", len(expectedConf.Protocols), len(actualConf.Protocols))
	}
	for i := 0; i < len(expectedConf.Protocols); i++ {
		if expectedConf.Protocols[i] != actualConf.Protocols[i] {
			t.Errorf("index %d: expected protocol %s, actual protocol %s", i, expectedConf.Protocols[i], actualConf.Protocols[i])
		}
	}
	if len(expectedConf.AuthMethods) != len(actualConf.AuthMethods) {
		t.Fatalf("expected authMethods length %d, actual authMethods length %d", len(expectedConf.AuthMethods), len(actualConf.AuthMethods))
	}
	for i := 0; i < len(expectedConf.AuthMethods); i++ {
		if expectedConf.AuthMethods[i] != actualConf.AuthMethods[i] {
			t.Errorf("index %d: expected authMethod %s, actual authMethod %s", i, expectedConf.AuthMethods[i], actualConf.AuthMethods[i])
		}
	}
	if len(expectedConf.LogDestinations) != len(actualConf.LogDestinations) {
		t.Fatalf("expected logDestinations length %d, actual logDestinations length %d", len(expectedConf.LogDestinations), len(actualConf.LogDestinations))
	}
	for i := 0; i < len(expectedConf.LogDestinations); i++ {
		if expectedConf.LogDestinations[i] != actualConf.LogDestinations[i] {
			t.Errorf("index %d: expected logDestination %s, actual logDestination %s", i, expectedConf.LogDestinations[i], actualConf.LogDestinations[i])
		}
	}
}
