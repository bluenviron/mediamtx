package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// SessionState represents the state of a client session
type SessionState string

const (
	SessionStateNew         SessionState = "new"
	SessionStateWaitingOffer SessionState = "waiting_offer"
	SessionStateOfferSent   SessionState = "offer_sent"
	SessionStateWaitingAnswer SessionState = "waiting_answer"
	SessionStateAnswerReceived SessionState = "answer_received"
	SessionStateConnected   SessionState = "connected"
	SessionStateFailed      SessionState = "failed"
	SessionStateClosed      SessionState = "closed"
)

// ClientSession represents an active client session
type ClientSession struct {
	ClientID    string
	Brand       string
	Serial      string
	StreamPath  string
	State       SessionState
	WHEPSession *WHEPSession
	CreatedAt   time.Time
	UpdatedAt   time.Time
	OfferSDP    string
	AnswerSDP   string
}

// SessionManager manages client sessions
type SessionManager struct {
	sessions    map[string]*ClientSession // key: clientID
	sessionsMu  sync.RWMutex
	maxSessions int
}

// NewSessionManager creates a new session manager
func NewSessionManager(maxSessions int) *SessionManager {
	if maxSessions <= 0 {
		maxSessions = 100
	}
	return &SessionManager{
		sessions:    make(map[string]*ClientSession),
		maxSessions: maxSessions,
	}
}

// CreateSession creates a new client session
func (sm *SessionManager) CreateSession(clientID, brand, serial string) (*ClientSession, error) {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()
	
	if len(sm.sessions) >= sm.maxSessions {
		return nil, fmt.Errorf("max sessions reached: %d", sm.maxSessions)
	}
	
	// Build StreamPath without leading slash. If brand is empty (serial-only topics),
	// use a default prefix 'fpt' to match MediaMTX expected paths like 'fpt/<serial>'.
	streamPath := ""
	if brand == "" {
		streamPath = fmt.Sprintf("fpt/%s", serial)
	} else {
		streamPath = fmt.Sprintf("%s/%s", brand, serial)
	}

	session := &ClientSession{
		ClientID:   clientID,
		Brand:      brand,
		Serial:     serial,
		StreamPath: streamPath,
		State:      SessionStateNew,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	sm.sessions[clientID] = session
	return session, nil
}

// GetSession gets a session by client ID
func (sm *SessionManager) GetSession(clientID string) *ClientSession {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()
	return sm.sessions[clientID]
}

// UpdateSessionState updates the state of a session
func (sm *SessionManager) UpdateSessionState(clientID string, state SessionState) error {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()
	
	session, ok := sm.sessions[clientID]
	if !ok {
		return fmt.Errorf("session not found: %s", clientID)
	}
	
	session.State = state
	session.UpdatedAt = time.Now()
	return nil
}

// UpdateSessionWHEP updates the WHEP session
func (sm *SessionManager) UpdateSessionWHEP(clientID string, whepSession *WHEPSession) error {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()
	
	session, ok := sm.sessions[clientID]
	if !ok {
		return fmt.Errorf("session not found: %s", clientID)
	}
	
	session.WHEPSession = whepSession
	session.UpdatedAt = time.Now()
	return nil
}

// DeleteSession deletes a session
func (sm *SessionManager) DeleteSession(clientID string) {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()
	delete(sm.sessions, clientID)
}

// GetSessionCount returns the number of active sessions
func (sm *SessionManager) GetSessionCount() int {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()
	return len(sm.sessions)
}

// GetSessionsBySerial returns all sessions for a specific serial
func (sm *SessionManager) GetSessionsBySerial(serial string) []*ClientSession {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()
	
	var sessions []*ClientSession
	for _, session := range sm.sessions {
		if session.Serial == serial {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// CleanupStaleSessions removes sessions older than the specified duration
func (sm *SessionManager) CleanupStaleSessions(maxAge time.Duration) int {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()
	
	now := time.Now()
	count := 0
	
	for clientID, session := range sm.sessions {
		if now.Sub(session.UpdatedAt) > maxAge {
			delete(sm.sessions, clientID)
			count++
		}
	}
	
	return count
}

// AdapterConfig holds the adapter configuration
type AdapterConfig struct {
	MQTT           MQTTConfig
	WHEP           WHEPConfig
	MediaMTXAPIURL string        // MediaMTX REST API URL (default: http://localhost:9997)
	MaxSessions    int
	SessionTimeout time.Duration
	LogLevel       string
}

// DefaultAdapterConfig returns default adapter configuration
func DefaultAdapterConfig() AdapterConfig {
	return AdapterConfig{
		MQTT:           DefaultMQTTConfig(),
		WHEP:           DefaultWHEPConfig(),
		MediaMTXAPIURL: "http://localhost:9997",
		MaxSessions:    100,
		SessionTimeout: 30 * time.Minute,
		LogLevel:       "info",
	}
}

// CameraCredentials stores credentials for a camera
type CameraCredentials struct {
	Serial   string
	Username string
	Password string
	RTSPURL  string // constructed RTSP URL
}

// Adapter is the main adapter that bridges FPT Camera signaling with MediaMTX
type Adapter struct {
	config         AdapterConfig
	mqtt           *MQTTClient
	whep           *WHEPClient
	sessions       *SessionManager
	credentials    map[string]*CameraCredentials // key: serial
	credentialsMu  sync.RWMutex
	running        bool
	runningMu      sync.RWMutex
	stopChan       chan struct{}
	apiServer      *APIServer
	
	// Callbacks
	OnClientConnected    func(clientID, serial string)
	OnClientDisconnected func(clientID, serial string)
	OnError              func(err error)
}

// NewAdapter creates a new adapter
func NewAdapter(config AdapterConfig) *Adapter {
	return &Adapter{
		config:      config,
		mqtt:        NewMQTTClient(config.MQTT),
		whep:        NewWHEPClient(config.WHEP),
		sessions:    NewSessionManager(config.MaxSessions),
		credentials: make(map[string]*CameraCredentials),
		stopChan:    make(chan struct{}),
	}
}

// Start starts the adapter
func (a *Adapter) Start() error {
	a.runningMu.Lock()
	if a.running {
		a.runningMu.Unlock()
		return fmt.Errorf("adapter already running")
	}
	a.running = true
	a.runningMu.Unlock()
	
	// Setup MQTT callbacks
	a.mqtt.OnConnected = func() {
		a.log("Connected to MQTT broker")
		a.subscribeToTopics()
	}
	
	a.mqtt.OnDisconnected = func(err error) {
		a.log("Disconnected from MQTT broker: %v", err)
	}
	
	// Connect to MQTT
	if err := a.mqtt.Connect(); err != nil {
		return fmt.Errorf("failed to connect to MQTT: %w", err)
	}
	
	// Start API server for ONVIF discovery
	a.apiServer = NewAPIServer(a, 8890)
	if err := a.apiServer.Start(); err != nil {
		a.logError("Failed to start API server: %v", err)
		// Non-fatal, continue without API
	}
	
	// Start cleanup goroutine
	go a.cleanupLoop()
	
	a.log("Adapter started")
	return nil
}

// Stop stops the adapter
func (a *Adapter) Stop() {
	a.runningMu.Lock()
	if !a.running {
		a.runningMu.Unlock()
		return
	}
	a.running = false
	a.runningMu.Unlock()
	
	close(a.stopChan)
	
	// Stop API server
	if a.apiServer != nil {
		a.apiServer.Stop()
	}
	
	a.mqtt.Disconnect()
	a.log("Adapter stopped")
}

// subscribeToTopics subscribes to all required MQTT topics
func (a *Adapter) subscribeToTopics() {
	// Subscribe to all signaling requests (both brand/serial and serial-only formats)
	topic1 := a.mqtt.Topics().RequestSignalingWildcard()
	if err := a.mqtt.Subscribe(topic1, a.handleSignalingRequest); err != nil {
		a.logError("Failed to subscribe to signaling (brand/serial): %v", err)
	}

	topic2 := a.mqtt.Topics().RequestSignalingWildcardSingle()
	if err := a.mqtt.Subscribe(topic2, a.handleSignalingRequest); err != nil {
		a.logError("Failed to subscribe to signaling (serial-only): %v", err)
	}

	// Subscribe to all credentials (both formats)
	cred1 := a.mqtt.Topics().CredentialWildcard()
	if err := a.mqtt.Subscribe(cred1, a.handleCredential); err != nil {
		a.logError("Failed to subscribe to credentials (brand/serial): %v", err)
	}

	cred2 := a.mqtt.Topics().CredentialWildcardSingle()
	if err := a.mqtt.Subscribe(cred2, a.handleCredential); err != nil {
		a.logError("Failed to subscribe to credentials (serial-only): %v", err)
	}

	// Subscribe to adapter control topic for add_source commands
	controlTopic := "fpt/adapter/control"
	if err := a.mqtt.Subscribe(controlTopic, a.handleControlMessage); err != nil {
		a.logError("Failed to subscribe to control topic: %v", err)
	}

	a.log("Subscribed to topics: %s, %s, %s, %s, %s", topic1, topic2, cred1, cred2, controlTopic)
}

// handleSignalingRequest handles incoming signaling requests from clients
func (a *Adapter) handleSignalingRequest(topic string, payload []byte) {
	a.log("Received signaling request on topic: %s", topic)
	
	// Parse topic to extract brand and serial
	brand, serial, err := a.parseSignalingTopic(topic)
	if err != nil {
		a.logError("Failed to parse topic: %v", err)
		return
	}
	
	// Parse the message
	var msg SignalingMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		a.logError("Failed to parse message: %v", err)
		return
	}
	
	// Get message type
	msgType, ok := msg.Data["Type"].(string)
	if !ok {
		a.logError("Missing Type in message data")
		return
	}
	
	clientID, _ := msg.Data["ClientId"].(string)
	
	switch SignalingType(msgType) {
	case SignalingTypeRequest:
		a.handleInitialRequest(brand, serial, clientID)
		
	case SignalingTypeAnswer:
		sdp, _ := msg.Data["Sdp"].(string)
		a.handleAnswer(brand, serial, clientID, sdp)
		
	case SignalingTypeIceCandidate:
		a.handleIceCandidate(brand, serial, clientID, msg.Data)
		
	default:
		a.log("Unknown signaling type: %s", msgType)
	}
}

// parseSignalingTopic parses the signaling topic to extract brand and serial
func (a *Adapter) parseSignalingTopic(topic string) (brand, serial string, err error) {
	// Use TopicBuilder to parse topic based on configured prefix
	brand, serial, msgType, parseErr := a.mqtt.Topics().ParseTopic(topic)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid topic format: %s", topic)
	}
	if msgType != "request/signaling" {
		return "", "", fmt.Errorf("unexpected message type: %s", msgType)
	}
	return brand, serial, nil
}

// handleInitialRequest handles the initial signaling request from a client
func (a *Adapter) handleInitialRequest(brand, serial, clientID string) {
	a.log("Handling initial request from client %s for %s", clientID, serial)
	
	// Check if max sessions reached
	if a.sessions.GetSessionCount() >= a.config.MaxSessions {
		// Send deny response
		denyResp := NewDenyResponse(serial, clientID, a.config.MaxSessions, a.sessions.GetSessionCount())
		if err := a.mqtt.PublishSignalingResponse(brand, serial, denyResp); err != nil {
			a.logError("Failed to send deny response: %v", err)
		}
		return
	}
	
	// Create session
	session, err := a.sessions.CreateSession(clientID, brand, serial)
	if err != nil {
		a.logError("Failed to create session: %v", err)
		return
	}

	a.log("Created session for client %s -> stream %s", clientID, session.StreamPath)
	
	// Get offer from MediaMTX via WHEP
	// For standard WHEP, client sends offer and server responds with answer
	// But FPT protocol expects server to send offer first
	// So we need to generate an offer ourselves or use MediaMTX's special handling
	
	// For now, we'll try to get stream info and create an offer
	streamPath := session.StreamPath
	
	// Check if stream is available
	if !a.whep.CheckStreamReady(streamPath) {
		a.log("Stream not ready: %s", streamPath)
		// Still send offer, MediaMTX might handle it
	}
	
	// Create a basic SDP offer
	// In real implementation, this would come from WebRTC peer connection
	// For now, we'll request MediaMTX to generate one or use a template
	
	whepSession, err := a.whep.CreateOffer(streamPath)
	if err != nil {
		a.logError("Failed to get offer from MediaMTX: %v", err)
		// Try alternative approach - send a placeholder and wait for client offer
		a.sendOfferRequest(brand, serial, clientID)
		return
	}
	
	// Update session
	session.WHEPSession = whepSession
	session.OfferSDP = whepSession.AnswerSDP // MediaMTX sends SDP in response
	a.sessions.UpdateSessionState(clientID, SessionStateOfferSent)
	
	// Send offer to client via MQTT
	iceServers := a.whep.GetICEServers()
	offerResp := NewOfferResponse(serial, clientID, whepSession.AnswerSDP, iceServers)
	
	if err := a.mqtt.PublishSignalingResponse(brand, serial, offerResp); err != nil {
		a.logError("Failed to send offer response: %v", err)
		return
	}
	
	a.log("Sent offer to client %s", clientID)
	
	if a.OnClientConnected != nil {
		a.OnClientConnected(clientID, serial)
	}
}

// sendOfferRequest sends a request for client to send offer (alternative flow)
func (a *Adapter) sendOfferRequest(brand, serial, clientID string) {
	
	iceServers := a.whep.GetICEServers()
	
	// Send a special response indicating client should send offer
	resp := &SignalingResponse{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: ResponseData{
			Type:       "request_offer",
			ClientID:   clientID,
			IceServers: iceServers,
		},
		Timestamp: time.Now().Unix(),
		Result: Result{
			Ret:     ResultSuccess,
			Message: "Please send offer",
		},
	}
	
	a.mqtt.PublishSignalingResponse(brand, serial, resp)
}

// handleAnswer handles the SDP answer from client
func (a *Adapter) handleAnswer(brand, serial, clientID, sdp string) {
	a.log("Handling answer from client %s", clientID)
	
	session := a.sessions.GetSession(clientID)
	if session == nil {
		a.logError("Session not found for client: %s", clientID)
		return
	}
	
	session.AnswerSDP = sdp
	
	// If we have a WHEP session, send the answer to MediaMTX
	if session.WHEPSession != nil && session.WHEPSession.SessionURL != "" {
		if err := a.whep.SendAnswer(session.WHEPSession.SessionURL, sdp); err != nil {
			a.logError("Failed to send answer to MediaMTX: %v", err)
			a.sessions.UpdateSessionState(clientID, SessionStateFailed)
			return
		}
	} else {
		// Client sent offer, now we send to MediaMTX and get answer
		whepSession, err := a.whep.SendOffer(session.StreamPath, sdp)
		if err != nil {
			a.logError("Failed to send offer to MediaMTX: %v", err)
			a.sessions.UpdateSessionState(clientID, SessionStateFailed)
			return
		}
		
		session.WHEPSession = whepSession
		a.sessions.UpdateSessionWHEP(clientID, whepSession)
		
		// Send answer back to client
		iceServers := a.whep.GetICEServers()
		offerResp := NewOfferResponse(serial, clientID, whepSession.AnswerSDP, iceServers)
		offerResp.Data.Type = SignalingTypeAnswer
		
		if err := a.mqtt.PublishSignalingResponse(brand, serial, offerResp); err != nil {
			a.logError("Failed to send answer response: %v", err)
			return
		}
	}
	
	a.sessions.UpdateSessionState(clientID, SessionStateConnected)
	
	// Send CCU update
	ccuResp := NewCCUResponse(serial, a.sessions.GetSessionCount())
	a.mqtt.PublishSignalingResponse(brand, serial, ccuResp)
	
	a.log("Client %s connected successfully", clientID)
}

// handleIceCandidate handles ICE candidates from client
func (a *Adapter) handleIceCandidate(brand, serial, clientID string, data map[string]interface{}) {
	// brand and serial may not be needed here; mark as used to avoid compiler warnings
	_ = brand
	_ = serial
	a.log("Handling ICE candidate from client %s", clientID)
	
	session := a.sessions.GetSession(clientID)
	if session == nil || session.WHEPSession == nil {
		a.logError("Session not found for ICE candidate: %s", clientID)
		return
	}
	
	// Parse candidate
	candidateData, ok := data["Candidate"].(map[string]interface{})
	if !ok {
		a.logError("Invalid candidate data")
		return
	}
	
	candidate := IceCandidate{
		Candidate:     candidateData["candidate"].(string),
		SDPMid:        candidateData["sdpMid"].(string),
	}
	if idx, ok := candidateData["sdpMLineIndex"].(float64); ok {
		candidate.SDPMLineIndex = int(idx)
	}
	
	// Send to MediaMTX
	if err := a.whep.AddICECandidate(session.WHEPSession.SessionURL, candidate, session.WHEPSession.ETag); err != nil {
		a.logError("Failed to add ICE candidate: %v", err)
	}
}

// handleCredential handles camera credentials from client
func (a *Adapter) handleCredential(topic string, payload []byte) {
	a.log("Received credential on topic: %s", topic)
	a.log("Credential payload: %s", string(payload))
	
	var msg CredentialMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		a.logError("Failed to parse credential message: %v", err)
		return
	}
	
	a.log("Received credentials for camera %s: user=%s, ip=%s", msg.Serial, msg.Data.Username, msg.Data.IP)
	
	// Get camera IP - either from message or use serial as hostname
	cameraIP := msg.Data.IP
	if cameraIP == "" {
		// If no IP provided, try using serial as hostname (might work in some networks)
		// Or you could implement ONVIF WS-Discovery here
		a.log("No IP provided, using serial as hostname: %s", msg.Serial)
		cameraIP = msg.Serial
	}
	
	var rtspURL string
	var err error
	
	// Try ONVIF to get RTSP URL (this will log all SOAP XML)
	// Use client with auth for HTTP Digest fallback
	a.log("Attempting ONVIF connection to %s...", cameraIP)
	onvif := NewONVIFClientWithAuth(a.log, msg.Data.Username, msg.Data.Password)
	rtspURL, err = onvif.GetRTSPURL(cameraIP, 80, msg.Data.Username, msg.Data.Password)
	
	if err != nil {
		a.logError("ONVIF failed: %v", err)
		// Fallback: construct default RTSP URL (Hikvision/FPT camera format)
		a.log("Falling back to default RTSP URL format")
		rtspURL = fmt.Sprintf("rtsp://%s:%s@%s:554/Streaming/Channels/101",
			msg.Data.Username, msg.Data.Password, cameraIP)
	} else {
		a.log("ONVIF success! Got RTSP URL: %s", rtspURL)
	}
	
	// Store credentials
	creds := &CameraCredentials{
		Serial:   msg.Serial,
		Username: msg.Data.Username,
		Password: msg.Data.Password,
		RTSPURL:  rtspURL,
	}
	
	a.credentialsMu.Lock()
	a.credentials[msg.Serial] = creds
	a.credentialsMu.Unlock()
	
	// Create path in MediaMTX with RTSP source
	streamPath := fmt.Sprintf("fpt/%s", msg.Serial)
	if err := a.whep.AddPath(streamPath, rtspURL); err != nil {
		a.logError("Failed to add path to MediaMTX: %v", err)
		// Don't return - path might already exist, continue anyway
	} else {
		a.log("Created MediaMTX path %s -> %s", streamPath, rtspURL)
	}
}

// cleanupLoop periodically cleans up stale sessions
func (a *Adapter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			count := a.sessions.CleanupStaleSessions(a.config.SessionTimeout)
			if count > 0 {
				a.log("Cleaned up %d stale sessions", count)
			}
		case <-a.stopChan:
			return
		}
	}
}

// GetSessionCount returns the number of active sessions
func (a *Adapter) GetSessionCount() int {
	return a.sessions.GetSessionCount()
}

// DisconnectClient disconnects a specific client
func (a *Adapter) DisconnectClient(clientID string) error {
	session := a.sessions.GetSession(clientID)
	if session == nil {
		return fmt.Errorf("session not found: %s", clientID)
	}
	
	// Delete WHEP session if exists
	if session.WHEPSession != nil && session.WHEPSession.SessionURL != "" {
		a.whep.DeleteSession(session.WHEPSession.SessionURL)
	}
	
	a.sessions.DeleteSession(clientID)
	
	if a.OnClientDisconnected != nil {
		a.OnClientDisconnected(clientID, session.Serial)
	}
	
	return nil
}

// ControlMessage represents a control message from the client
type ControlMessage struct {
	Action    string `json:"action"`
	Path      string `json:"path"`
	RTSPUrl   string `json:"rtsp_url"`
	Timestamp int64  `json:"timestamp"`
}

// handleControlMessage handles control messages for adapter operations
func (a *Adapter) handleControlMessage(topic string, payload []byte) {
	a.log("Received control message on topic: %s", topic)
	
	var msg ControlMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		a.logError("Failed to parse control message: %v", err)
		return
	}
	
	a.log("Control message: action=%s, path=%s, rtsp_url=%s", msg.Action, msg.Path, msg.RTSPUrl)
	
	switch msg.Action {
	case "add_source":
		a.handleAddSource(msg.Path, msg.RTSPUrl)
	case "remove_source":
		a.handleRemoveSource(msg.Path)
	default:
		a.logError("Unknown control action: %s", msg.Action)
	}
}

// handleAddSource adds an RTSP source to MediaMTX
func (a *Adapter) handleAddSource(path, rtspUrl string) {
	if path == "" || rtspUrl == "" {
		a.logError("add_source requires path and rtsp_url")
		return
	}
	
	a.log("Adding RTSP source: path=%s, url=%s", path, rtspUrl)
	
	// Use MediaMTX API to add the path with RTSP source
	err := a.addPathToMediaMTX(path, rtspUrl)
	if err != nil {
		a.logError("Failed to add source to MediaMTX: %v", err)
		return
	}
	
	a.log("Successfully added source: %s", path)
}

// handleRemoveSource removes a source from MediaMTX
func (a *Adapter) handleRemoveSource(path string) {
	if path == "" {
		a.logError("remove_source requires path")
		return
	}
	
	a.log("Removing source: path=%s", path)
	// TODO: Implement removal via MediaMTX API
}

// addPathToMediaMTX adds a path configuration to MediaMTX via its API
func (a *Adapter) addPathToMediaMTX(path, rtspUrl string) error {
	// MediaMTX API endpoint for adding/editing path config
	// POST /v3/config/paths/add/{name}
	// or PATCH /v3/config/paths/edit/{name}
	
	apiUrl := fmt.Sprintf("%s/v3/config/paths/add/%s", a.config.MediaMTXAPIURL, path)
	
	// Path configuration
	pathConfig := map[string]interface{}{
		"source": rtspUrl,
	}
	
	configJSON, err := json.Marshal(pathConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	a.log("Calling MediaMTX API: POST %s", apiUrl)
	a.log("Config: %s", string(configJSON))
	
	// Make HTTP request to MediaMTX API
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(configJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	a.log("MediaMTX API response: %d - %s", resp.StatusCode, string(body))
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("MediaMTX API returned %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// log logs a message
func (a *Adapter) log(format string, args ...interface{}) {
	fmt.Printf("[Adapter] "+format+"\n", args...)
}

// logError logs an error
func (a *Adapter) logError(format string, args ...interface{}) {
	fmt.Printf("[Adapter ERROR] "+format+"\n", args...)
	if a.OnError != nil {
		a.OnError(fmt.Errorf(format, args...))
	}
}
