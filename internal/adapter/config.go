package adapter

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfigFromEnv loads adapter configuration from environment variables
func LoadConfigFromEnv() AdapterConfig {
	config := DefaultAdapterConfig()
	
	// MQTT Configuration
	if v := os.Getenv("FPT_MQTT_BROKER"); v != "" {
		config.MQTT.BrokerURL = v
	}
	if v := os.Getenv("FPT_MQTT_USER"); v != "" {
		config.MQTT.Username = v
	}
	if v := os.Getenv("FPT_MQTT_PASS"); v != "" {
		config.MQTT.Password = v
	}
	if v := os.Getenv("FPT_MQTT_TOPIC_PREFIX"); v != "" {
		config.MQTT.TopicPrefix = v
	}
	if v := os.Getenv("FPT_MQTT_CLIENT_PREFIX"); v != "" {
		config.MQTT.ClientIDPrefix = v
	}
	if v := os.Getenv("FPT_MQTT_TLS_ENABLED"); v != "" {
		config.MQTT.TLSEnabled = v == "1" || strings.ToLower(v) == "true"
	}
	if v := os.Getenv("FPT_MQTT_TLS_INSECURE_SKIP_VERIFY"); v != "" {
		config.MQTT.TLSInsecureSkipVerify = v == "1" || strings.ToLower(v) == "true"
	}
	if v := os.Getenv("FPT_MQTT_QOS"); v != "" {
		if qos, err := strconv.Atoi(v); err == nil && qos >= 0 && qos <= 2 {
			config.MQTT.QoS = byte(qos)
		}
	}
	if v := os.Getenv("FPT_MQTT_KEEPALIVE"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			config.MQTT.KeepAlive = time.Duration(secs) * time.Second
		}
	}
	
	// WHEP Configuration
	if v := os.Getenv("MEDIAMTX_WHEP_URL"); v != "" {
		config.WHEP.BaseURL = v
	}
	if v := os.Getenv("WEBRTC_STUN_SERVERS"); v != "" {
		config.WHEP.STUNServers = strings.Split(v, ",")
	}
	if v := os.Getenv("TURN_SERVER_URL"); v != "" {
		config.WHEP.TURNServer = v
	}
	if v := os.Getenv("TURN_USERNAME"); v != "" {
		config.WHEP.TURNUsername = v
	}
	if v := os.Getenv("TURN_PASSWORD"); v != "" {
		config.WHEP.TURNPassword = v
	}
	
	// Session Configuration
	if v := os.Getenv("ADAPTER_MAX_SESSIONS"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			config.MaxSessions = max
		}
	}
	if v := os.Getenv("ADAPTER_SESSION_TIMEOUT"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil {
			config.SessionTimeout = time.Duration(mins) * time.Minute
		}
	}
	if v := os.Getenv("ADAPTER_LOG_LEVEL"); v != "" {
		config.LogLevel = v
	}
	
	return config
}

// EnvExample returns example environment variables
func EnvExample() string {
	return `# FPT Camera MQTT Configuration
FPT_MQTT_BROKER=wss://beta-broker-mqtt.fcam.vn:8084/mqtt
FPT_MQTT_USER=hoangbd7
FPT_MQTT_PASS=Hoangbd7
FPT_MQTT_CLIENT_PREFIX=mediamtx-adapter-
FPT_MQTT_TLS_ENABLED=1
FPT_MQTT_TLS_INSECURE_SKIP_VERIFY=1
FPT_MQTT_QOS=1
FPT_MQTT_KEEPALIVE=60

# MediaMTX WHEP Configuration
MEDIAMTX_WHEP_URL=http://localhost:8889

# WebRTC ICE Servers
WEBRTC_STUN_SERVERS=stun:stun-connect.fcam.vn:3478,stun:stunp-connect.fcam.vn:3478
TURN_SERVER_URL=turn:turn-connect.fcam.vn:3478
TURN_USERNAME=turnuser
TURN_PASSWORD=camfptvnturn133099

# Adapter Configuration
ADAPTER_MAX_SESSIONS=100
ADAPTER_SESSION_TIMEOUT=30
ADAPTER_LOG_LEVEL=info
`
}
