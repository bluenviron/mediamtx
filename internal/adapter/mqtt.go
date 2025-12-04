package adapter

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTConfig holds MQTT connection configuration
type MQTTConfig struct {
	BrokerURL          string
	Username           string
	Password           string
	ClientIDPrefix     string
	TopicPrefix        string
	KeepAlive          time.Duration
	ConnectTimeout     time.Duration
	ReconnectInterval  time.Duration
	TLSEnabled         bool
	TLSInsecureSkipVerify bool
	QoS                byte
}

// DefaultMQTTConfig returns default MQTT configuration
func DefaultMQTTConfig() MQTTConfig {
	return MQTTConfig{
		BrokerURL:          "wss://beta-broker-mqtt.fcam.vn:8084/mqtt",
		Username:           "",
		Password:           "",
		ClientIDPrefix:     "mediamtx-adapter-",
		TopicPrefix:        "ipc/fss",
		KeepAlive:          60 * time.Second,
		ConnectTimeout:     30 * time.Second,
		ReconnectInterval:  5 * time.Second,
		TLSEnabled:         true,
		TLSInsecureSkipVerify: true,
		QoS:                1,
	}
}

// TopicBuilder builds MQTT topics for FPT Camera protocol
type TopicBuilder struct {
	prefix string
}

// NewTopicBuilder creates a new topic builder
func NewTopicBuilder(prefix string) *TopicBuilder {
	if prefix == "" {
		prefix = "ipc"
	}
	return &TopicBuilder{prefix: prefix}
}

// Discovery returns the discovery topic
func (tb *TopicBuilder) Discovery() string {
	return fmt.Sprintf("%s/discovery", tb.prefix)
}

// RequestSignaling returns the request signaling topic for a camera
func (tb *TopicBuilder) RequestSignaling(brand, serial string) string {
	return fmt.Sprintf("%s/%s/%s/request/signaling", tb.prefix, brand, serial)
}

// ResponseSignaling returns the response signaling topic for a camera
func (tb *TopicBuilder) ResponseSignaling(brand, serial string) string {
	return fmt.Sprintf("%s/%s/%s/response/signaling", tb.prefix, brand, serial)
}

// Credential returns the credential topic for a camera
func (tb *TopicBuilder) Credential(brand, serial string) string {
	return fmt.Sprintf("%s/%s/%s/credential", tb.prefix, brand, serial)
}

// RequestSignalingWildcard returns wildcard topic for all request signaling
func (tb *TopicBuilder) RequestSignalingWildcard() string {
	return fmt.Sprintf("%s/+/+/request/signaling", tb.prefix)
}

// CredentialWildcard returns wildcard topic for all credentials
func (tb *TopicBuilder) CredentialWildcard() string {
	return fmt.Sprintf("%s/+/+/credential", tb.prefix)
}

// RequestSignalingWildcardSingle returns wildcard topic assuming client uses only serial segment
func (tb *TopicBuilder) RequestSignalingWildcardSingle() string {
    return fmt.Sprintf("%s/+/request/signaling", tb.prefix)
}

// CredentialWildcardSingle returns wildcard topic assuming client uses only serial segment
func (tb *TopicBuilder) CredentialWildcardSingle() string {
    return fmt.Sprintf("%s/+/credential", tb.prefix)
}

// ParseTopic parses a topic and extracts brand and serial
func (tb *TopicBuilder) ParseTopic(topic string) (brand, serial, messageType string, err error) {
	// Split topic and prefix into parts
	parts := splitTopic(topic)
	prefixParts := splitTopic(tb.prefix)

	if len(parts) < len(prefixParts)+1 {
		return "", "", "", fmt.Errorf("invalid topic format: %s", topic)
	}

	// Verify prefix matches the beginning of the topic; if so, remove those segments
	matches := true
	for i := 0; i < len(prefixParts); i++ {
		if parts[i] != prefixParts[i] {
			matches = false
			break
		}
	}
	if !matches {
		return "", "", "", fmt.Errorf("topic does not start with expected prefix: %s", topic)
	}

	parts = parts[len(prefixParts):]

	if len(parts) == 3 {
		// serial-only format: <serial>/<type>/<subtype>
		serial = parts[0]
		brand = ""
		messageType = parts[1] + "/" + parts[2]
		return brand, serial, messageType, nil
	}

	if len(parts) == 4 {
		// brand/serial format: <brand>/<serial>/<type>/<subtype>
		brand = parts[0]
		serial = parts[1]
		messageType = parts[2] + "/" + parts[3]
		return brand, serial, messageType, nil
	}

	return "", "", "", fmt.Errorf("invalid topic format: %s", topic)
}

// MQTTClient wraps the MQTT client for FPT Camera signaling
type MQTTClient struct {
	config       MQTTConfig
	client       mqtt.Client
	topics       *TopicBuilder
	handlers     map[string]MessageHandler
	handlersMu   sync.RWMutex
	connected    bool
	connectedMu  sync.RWMutex
	
	// Callbacks
	OnConnected    func()
	OnDisconnected func(error)
	OnError        func(error)
}

// MessageHandler is a callback for handling MQTT messages
type MessageHandler func(topic string, payload []byte)

// NewMQTTClient creates a new MQTT client
func NewMQTTClient(config MQTTConfig) *MQTTClient {
	return &MQTTClient{
		config:   config,
		topics:   NewTopicBuilder(config.TopicPrefix),
		handlers: make(map[string]MessageHandler),
	}
}

// Topics returns the topic builder
func (mc *MQTTClient) Topics() *TopicBuilder {
	return mc.topics
}

// Connect connects to the MQTT broker
func (mc *MQTTClient) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mc.config.BrokerURL)
	opts.SetClientID(mc.config.ClientIDPrefix + generateClientID())
	opts.SetUsername(mc.config.Username)
	opts.SetPassword(mc.config.Password)
	opts.SetKeepAlive(mc.config.KeepAlive)
	opts.SetConnectTimeout(mc.config.ConnectTimeout)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetryInterval(mc.config.ReconnectInterval)
	opts.SetCleanSession(true)

	// TLS configuration
	if mc.config.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: mc.config.TLSInsecureSkipVerify,
		}
		opts.SetTLSConfig(tlsConfig)
	}

	// Connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		mc.connectedMu.Lock()
		mc.connected = true
		mc.connectedMu.Unlock()
		
		// Resubscribe to all topics
		mc.resubscribeAll()
		
		if mc.OnConnected != nil {
			mc.OnConnected()
		}
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		mc.connectedMu.Lock()
		mc.connected = false
		mc.connectedMu.Unlock()
		
		if mc.OnDisconnected != nil {
			mc.OnDisconnected(err)
		}
	})

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		mc.handleMessage(msg.Topic(), msg.Payload())
	})

	mc.client = mqtt.NewClient(opts)
	
	token := mc.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return nil
}

// Disconnect disconnects from the MQTT broker
func (mc *MQTTClient) Disconnect() {
	if mc.client != nil && mc.client.IsConnected() {
		mc.client.Disconnect(250)
	}
}

// IsConnected returns true if connected to the broker
func (mc *MQTTClient) IsConnected() bool {
	mc.connectedMu.RLock()
	defer mc.connectedMu.RUnlock()
	return mc.connected
}

// Subscribe subscribes to a topic with a message handler
func (mc *MQTTClient) Subscribe(topic string, handler MessageHandler) error {
	mc.handlersMu.Lock()
	mc.handlers[topic] = handler
	mc.handlersMu.Unlock()

	token := mc.client.Subscribe(topic, mc.config.QoS, func(client mqtt.Client, msg mqtt.Message) {
		mc.handleMessage(msg.Topic(), msg.Payload())
	})
	
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", topic, token.Error())
	}
	
	return nil
}

// Unsubscribe unsubscribes from a topic
func (mc *MQTTClient) Unsubscribe(topic string) error {
	mc.handlersMu.Lock()
	delete(mc.handlers, topic)
	mc.handlersMu.Unlock()

	token := mc.client.Unsubscribe(topic)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to unsubscribe from %s: %w", topic, token.Error())
	}
	
	return nil
}

// Publish publishes a message to a topic
func (mc *MQTTClient) Publish(topic string, payload interface{}, retain bool) error {
	var data []byte
	var err error

	switch v := payload.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	token := mc.client.Publish(topic, mc.config.QoS, retain, data)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, token.Error())
	}
	
	return nil
}

// PublishSignalingResponse publishes a signaling response
func (mc *MQTTClient) PublishSignalingResponse(brand, serial string, response interface{}) error {
	topic := mc.topics.ResponseSignaling(brand, serial)
	return mc.Publish(topic, response, false)
}

// handleMessage routes messages to registered handlers
func (mc *MQTTClient) handleMessage(topic string, payload []byte) {
	mc.handlersMu.RLock()
	
	// Check for exact match first
	if handler, ok := mc.handlers[topic]; ok {
		mc.handlersMu.RUnlock()
		handler(topic, payload)
		return
	}
	
	// Check for wildcard matches
	for pattern, handler := range mc.handlers {
		if matchTopic(pattern, topic) {
			mc.handlersMu.RUnlock()
			handler(topic, payload)
			return
		}
	}
	
	mc.handlersMu.RUnlock()
}

// resubscribeAll resubscribes to all topics after reconnection
func (mc *MQTTClient) resubscribeAll() {
	mc.handlersMu.RLock()
	topics := make([]string, 0, len(mc.handlers))
	for topic := range mc.handlers {
		topics = append(topics, topic)
	}
	mc.handlersMu.RUnlock()

	for _, topic := range topics {
		mc.client.Subscribe(topic, mc.config.QoS, func(client mqtt.Client, msg mqtt.Message) {
			mc.handleMessage(msg.Topic(), msg.Payload())
		})
	}
}

// matchTopic checks if a topic matches a pattern with wildcards
func matchTopic(pattern, topic string) bool {
	// Simple wildcard matching for + and #
	// This is a simplified implementation
	patternParts := splitTopic(pattern)
	topicParts := splitTopic(topic)

	if len(patternParts) != len(topicParts) {
		// Check for # wildcard at the end
		if len(patternParts) > 0 && patternParts[len(patternParts)-1] == "#" {
			return len(topicParts) >= len(patternParts)-1
		}
		return false
	}

	for i, part := range patternParts {
		if part == "+" {
			continue
		}
		if part == "#" {
			return true
		}
		if part != topicParts[i] {
			return false
		}
	}

	return true
}

// splitTopic splits a topic into parts
func splitTopic(topic string) []string {
	var parts []string
	current := ""
	for _, c := range topic {
		if c == '/' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
