package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WHEPConfig holds WHEP client configuration
type WHEPConfig struct {
	BaseURL        string
	Timeout        time.Duration
	ICEServers     []string
	TURNServer     string
	TURNUsername   string
	TURNPassword   string
	STUNServers    []string
}

// DefaultWHEPConfig returns default WHEP configuration
func DefaultWHEPConfig() WHEPConfig {
	return WHEPConfig{
		BaseURL:      "http://localhost:8889",
		Timeout:      30 * time.Second,
		ICEServers:   []string{},
		STUNServers:  []string{"stun:stun-connect.fcam.vn:3478", "stun:stunp-connect.fcam.vn:3478"},
		TURNServer:   "turn:turn-connect.fcam.vn:3478",
		TURNUsername: "turnuser",
		TURNPassword: "camfptvnturn133099",
	}
}

// WHEPClient handles communication with MediaMTX WHEP endpoints
type WHEPClient struct {
	config     WHEPConfig
	httpClient *http.Client
}

// NewWHEPClient creates a new WHEP client
func NewWHEPClient(config WHEPConfig) *WHEPClient {
	return &WHEPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// WHEPSession represents an active WHEP session
type WHEPSession struct {
	SessionURL string
	AnswerSDP  string
	ETag       string
}

// GetICEServers returns the configured ICE servers as a formatted list
func (wc *WHEPClient) GetICEServers() []string {
	servers := make([]string, 0)
	
	// Add STUN servers
	for _, stun := range wc.config.STUNServers {
		// Extract just the host for the FPT format
		host := strings.TrimPrefix(stun, "stun:")
		servers = append(servers, host)
	}
	
	// Add TURN server
	if wc.config.TURNServer != "" {
		host := strings.TrimPrefix(wc.config.TURNServer, "turn:")
		servers = append(servers, host)
	}
	
	return servers
}

// CreateOffer sends a WHEP request to get an SDP offer from MediaMTX
// This is used when we need MediaMTX to generate the offer (server-initiated)
func (wc *WHEPClient) CreateOffer(streamPath string) (*WHEPSession, error) {
	url := fmt.Sprintf("%s/%s/whep", wc.config.BaseURL, streamPath)
	
	// For WHEP, we send an empty body or minimal SDP to get server offer
	// However, standard WHEP expects client to send offer
	// MediaMTX might have specific handling for this
	
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/sdp")
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("WHEP request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	answerSDP, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	session := &WHEPSession{
		SessionURL: resp.Header.Get("Location"),
		AnswerSDP:  string(answerSDP),
		ETag:       resp.Header.Get("ETag"),
	}
	
	return session, nil
}

// SendOffer sends a client's SDP offer to MediaMTX and gets the answer
// This follows standard WHEP flow where client sends offer
func (wc *WHEPClient) SendOffer(streamPath string, offerSDP string) (*WHEPSession, error) {
	url := fmt.Sprintf("%s/%s/whep", wc.config.BaseURL, streamPath)
	
	req, err := http.NewRequest("POST", url, strings.NewReader(offerSDP))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Accept", "application/sdp")
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("WHEP request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	answerSDP, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	session := &WHEPSession{
		SessionURL: resp.Header.Get("Location"),
		AnswerSDP:  string(answerSDP),
		ETag:       resp.Header.Get("ETag"),
	}
	
	return session, nil
}

// SendAnswer sends the client's answer to complete the WHEP handshake
// Used in cases where server sends offer first
func (wc *WHEPClient) SendAnswer(sessionURL string, answerSDP string) error {
	req, err := http.NewRequest("PATCH", sessionURL, strings.NewReader(answerSDP))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/sdp")
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("WHEP PATCH failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// AddICECandidate adds an ICE candidate to the session via WHEP
func (wc *WHEPClient) AddICECandidate(sessionURL string, candidate IceCandidate, eTag string) error {
	// Format ICE candidate for WHEP
	// MediaMTX expects specific format
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("failed to marshal candidate: %w", err)
	}
	
	req, err := http.NewRequest("PATCH", sessionURL, bytes.NewReader(candidateJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
	if eTag != "" {
		req.Header.Set("If-Match", eTag)
	}
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ICE candidate failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// DeleteSession deletes a WHEP session
func (wc *WHEPClient) DeleteSession(sessionURL string) error {
	req, err := http.NewRequest("DELETE", sessionURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// GetStreamInfo gets information about a stream from MediaMTX API
func (wc *WHEPClient) GetStreamInfo(streamPath string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/v3/paths/get/%s", strings.TrimSuffix(wc.config.BaseURL, "/whep"), streamPath)
	
	resp, err := wc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stream not found or not ready")
	}
	
	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return info, nil
}

// CheckStreamReady checks if a stream is ready for playback
func (wc *WHEPClient) CheckStreamReady(streamPath string) bool {
	info, err := wc.GetStreamInfo(streamPath)
	if err != nil {
		return false
	}
	
	// Check if stream has readers or is ready
	if ready, ok := info["ready"].(bool); ok {
		return ready
	}
	
	return true // Assume ready if we got info
}

// AddPath creates a new path configuration in MediaMTX with RTSP source
func (wc *WHEPClient) AddPath(streamPath, rtspURL string) error {
	// Use API port 9997 (default MediaMTX API)
	apiBase := strings.Replace(wc.config.BaseURL, ":8889", ":9997", 1)
	url := fmt.Sprintf("%s/v3/config/paths/add/%s", apiBase, streamPath)
	
	pathConf := map[string]interface{}{
		"source":         rtspURL,
		"sourceOnDemand": true,
	}
	
	jsonData, err := json.Marshal(pathConf)
	if err != nil {
		return fmt.Errorf("failed to marshal path config: %w", err)
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add path failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// DeletePath removes a path configuration from MediaMTX
func (wc *WHEPClient) DeletePath(streamPath string) error {
	apiBase := strings.Replace(wc.config.BaseURL, ":8889", ":9997", 1)
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", apiBase, streamPath)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := wc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete path failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}
