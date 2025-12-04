package adapter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIServer provides REST API for the adapter
type APIServer struct {
	adapter    *Adapter
	port       int
	server     *http.Server
	
	// Discovery cache
	discoveredDevices []DiscoveredDevice
	discoveryMu       sync.RWMutex
	lastDiscovery     time.Time
}

// DiscoveredDevice represents an ONVIF device found on network
type DiscoveredDevice struct {
	IP           string   `json:"ip"`
	Port         int      `json:"port"`
	XAddr        string   `json:"xaddr"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	Serial       string   `json:"serial,omitempty"`
	Firmware     string   `json:"firmware,omitempty"`
	Profiles     []ProfileInfo `json:"profiles,omitempty"`
	Services     []string `json:"services,omitempty"`
	HasMedia     bool     `json:"hasMedia"`
	DiscoveredAt string   `json:"discoveredAt"`
}

// ProfileInfo contains stream profile information
type ProfileInfo struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	RTSPPath string `json:"rtspPath,omitempty"` // Path without credentials
}

// DiscoverResponse is the API response for /api/discover
type DiscoverResponse struct {
	Success    bool              `json:"success"`
	Message    string            `json:"message,omitempty"`
	Devices    []DiscoveredDevice `json:"devices"`
	ScanTime   string            `json:"scanTime"`
	Duration   string            `json:"duration"`
}

// AuthRequest is the request body for /api/auth
type AuthRequest struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse is the API response for /api/auth
type AuthResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	RTSPURL   string   `json:"rtspUrl,omitempty"`
	RTSPURLs  []string `json:"rtspUrls,omitempty"` // Multiple profiles
	Profiles  []ProfileInfo `json:"profiles,omitempty"`
}

// NewAPIServer creates a new API server
func NewAPIServer(adapter *Adapter, port int) *APIServer {
	return &APIServer{
		adapter: adapter,
		port:    port,
	}
}

// Start starts the API server
func (s *APIServer) Start() error {
	mux := http.NewServeMux()
	
	// CORS middleware wrapper
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			h(w, r)
		}
	}
	
	// Routes
	mux.HandleFunc("/api/discover", corsHandler(s.handleDiscover))
	mux.HandleFunc("/api/auth", corsHandler(s.handleAuth))
	mux.HandleFunc("/api/devices", corsHandler(s.handleGetDevices))
	mux.HandleFunc("/api/health", corsHandler(s.handleHealth))
	
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}
	
	s.adapter.log("API server starting on port %d", s.port)
	
	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			s.adapter.logError("API server error: %v", err)
		}
	}()
	
	return nil
}

// Stop stops the API server
func (s *APIServer) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}

// handleHealth returns server health status
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleDiscover scans network for ONVIF devices
func (s *APIServer) handleDiscover(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	s.adapter.log("API: Starting network discovery...")
	startTime := time.Now()
	
	// Use ONVIF WS-Discovery
	onvif := NewONVIFClient(s.adapter.log)
	cameras, err := onvif.Discover(5 * time.Second)
	
	if err != nil {
		s.adapter.logError("Discovery failed: %v", err)
		json.NewEncoder(w).Encode(DiscoverResponse{
			Success:  false,
			Message:  err.Error(),
			Devices:  []DiscoveredDevice{},
			ScanTime: startTime.Format(time.RFC3339),
			Duration: time.Since(startTime).String(),
		})
		return
	}
	
	// Convert to DiscoveredDevice
	var devices []DiscoveredDevice
	for _, cam := range cameras {
		device := DiscoveredDevice{
			IP:           cam.IP,
			Port:         cam.Port,
			XAddr:        cam.XAddr,
			Serial:       cam.Serial,
			HasMedia:     false,
			DiscoveredAt: time.Now().Format(time.RFC3339),
		}
		
		// Try to get device info (without auth - some cameras allow this)
		if cam.XAddr != "" {
			info, err := onvif.GetDeviceInfo(cam.XAddr, "", "")
			if err == nil {
				device.Manufacturer = info["Manufacturer"]
				device.Model = info["Model"]
				device.Serial = info["SerialNumber"]
				device.Firmware = info["FirmwareVersion"]
			}
			
			// Get services to check for media
			services, err := onvif.GetServices(cam.XAddr, "", "")
			if err == nil {
				for ns := range services {
					device.Services = append(device.Services, ns)
					if containsMedia(ns) {
						device.HasMedia = true
					}
				}
			}
		}
		
		devices = append(devices, device)
	}
	
	// Cache results
	s.discoveryMu.Lock()
	s.discoveredDevices = devices
	s.lastDiscovery = time.Now()
	s.discoveryMu.Unlock()
	
	s.adapter.log("API: Discovery completed, found %d device(s)", len(devices))
	
	json.NewEncoder(w).Encode(DiscoverResponse{
		Success:  true,
		Message:  fmt.Sprintf("Found %d device(s)", len(devices)),
		Devices:  devices,
		ScanTime: startTime.Format(time.RFC3339),
		Duration: time.Since(startTime).String(),
	})
}

// handleGetDevices returns cached discovered devices
func (s *APIServer) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	s.discoveryMu.RLock()
	devices := s.discoveredDevices
	lastScan := s.lastDiscovery
	s.discoveryMu.RUnlock()
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices":  devices,
		"lastScan": lastScan.Format(time.RFC3339),
		"cached":   !lastScan.IsZero(),
	})
}

// handleAuth authenticates with a device and gets RTSP URLs
func (s *APIServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}
	
	if req.IP == "" {
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: "IP is required",
		})
		return
	}
	
	if req.Port == 0 {
		req.Port = 80
	}
	
	s.adapter.log("API: Authenticating with %s:%d (user: %s)", req.IP, req.Port, req.Username)
	
	// Create ONVIF client with credentials
	onvif := NewONVIFClientWithAuth(s.adapter.log, req.Username, req.Password)
	
	// Build device URL
	deviceURL := fmt.Sprintf("http://%s:%d/onvif/device_service", req.IP, req.Port)
	
	// Get services to find media URL
	services, err := onvif.GetServices(deviceURL, req.Username, req.Password)
	if err != nil {
		s.adapter.logError("Failed to get services: %v", err)

		// If the request port is 554 (RTSP) or we timed out contacting ONVIF,
		// try RTSP probe as a fallback (device might expose RTSP only)
		if req.Port == 554 {
			s.adapter.log("API: ONVIF failed on port 554; attempting RTSP probe fallback")
			rtspUrls := s.probeRTSP(req.IP, req.Port, req.Username, req.Password)
			if len(rtspUrls) > 0 {
				// store credential and return success
				serial := fmt.Sprintf("%s_%d", req.IP, req.Port)
				s.adapter.credentialsMu.Lock()
				s.adapter.credentials[serial] = &CameraCredentials{
					Serial:   serial,
					Username: req.Username,
					Password: req.Password,
					RTSPURL:  rtspUrls[0],
				}
				s.adapter.credentialsMu.Unlock()

				s.adapter.log("API: RTSP probe successful, found %d URL(s)", len(rtspUrls))
				json.NewEncoder(w).Encode(AuthResponse{
					Success:  true,
					Message:  fmt.Sprintf("RTSP probe found %d URL(s)", len(rtspUrls)),
					RTSPURL:  rtspUrls[0],
					RTSPURLs: rtspUrls,
				})
				return
			}
		}

		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to connect: %v", err),
		})
		return
	}
	
	// Find media service URL
	var mediaURL string
	for ns, url := range services {
		if containsMedia(ns) {
			mediaURL = url
			break
		}
	}
	
	if mediaURL == "" {
		mediaURL = fmt.Sprintf("http://%s:%d/onvif/media_service", req.IP, req.Port)
	}
	
	s.adapter.log("API: Media service URL: %s", mediaURL)
	
	// Get profiles
	profiles, err := onvif.GetProfiles(mediaURL, req.Username, req.Password)
	if err != nil {
		s.adapter.logError("Failed to get profiles: %v", err)
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: fmt.Sprintf("Authentication failed: %v", err),
		})
		return
	}
	
	if len(profiles) == 0 {
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: "No stream profiles found",
		})
		return
	}
	
	// Get stream URIs for each profile
	var rtspURLs []string
	var profileInfos []ProfileInfo
	
	for _, profile := range profiles {
		streamURI, err := onvif.GetStreamURI(mediaURL, req.Username, req.Password, profile.Token)
		if err != nil {
			s.adapter.log("Failed to get stream URI for profile %s: %v", profile.Token, err)
			continue
		}
		
		rtspURLs = append(rtspURLs, streamURI)
		profileInfos = append(profileInfos, ProfileInfo{
			Token:    profile.Token,
			Name:     profile.Name,
			RTSPPath: streamURI,
		})
		
		s.adapter.log("API: Profile %s (%s) -> %s", profile.Token, profile.Name, streamURI)
	}
	
	if len(rtspURLs) == 0 {
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: "Failed to get stream URLs",
		})
		return
	}
	
	// Store credentials in adapter
	serial := fmt.Sprintf("%s_%d", req.IP, req.Port)
	s.adapter.credentialsMu.Lock()
	s.adapter.credentials[serial] = &CameraCredentials{
		Serial:   serial,
		Username: req.Username,
		Password: req.Password,
		RTSPURL:  rtspURLs[0],
	}
	s.adapter.credentialsMu.Unlock()
	
	s.adapter.log("API: Authentication successful, found %d stream(s)", len(rtspURLs))
	
	json.NewEncoder(w).Encode(AuthResponse{
		Success:  true,
		Message:  fmt.Sprintf("Found %d stream profile(s)", len(rtspURLs)),
		RTSPURL:  rtspURLs[0],
		RTSPURLs: rtspURLs,
		Profiles: profileInfos,
	})
}

// probeRTSP tries common RTSP paths on the device and returns successful URLs
func (s *APIServer) probeRTSP(ip string, port int, username, password string) []string {
	commonPaths := []string{
		"/",
		"/cam/realmonitor?channel=1",
		"/cam/realmonitor?channel=1&subtype=0",
		"/Streaming/Channels/101",
		"/live.sdp",
	}

	var found []string
	for _, p := range commonPaths {
		ok, err := sendRTSPDescribe(ip, port, p, username, password)
		if err != nil {
			s.adapter.log("RTSP probe %s%s failed: %v", ip, p, err)
			continue
		}
		if ok {
			// build URL with credentials if provided
			url := fmt.Sprintf("rtsp://%s:%d%s", ip, port, p)
			if username != "" {
				// encode naive - assume no special chars
				url = fmt.Sprintf("rtsp://%s:%s@%s:%d%s", username, password, ip, port, p)
			}
			found = append(found, url)
		}
	}
	return found
}

// sendRTSPDescribe sends an RTSP DESCRIBE and handles Digest auth if required
func sendRTSPDescribe(ip string, port int, path, username, password string) (bool, error) {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := fmt.Sprintf("rtsp://%s:%d%s", ip, port, path)

	// send initial DESCRIBE
	req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\nAccept: application/sdp\r\n\r\n", uri)
	if _, err := conn.Write([]byte(req)); err != nil {
		return false, err
	}

	reader := bufio.NewReader(conn)
	// read status line
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	statusLine = strings.TrimSpace(statusLine)
	// Example: RTSP/1.0 401 Unauthorized
	if strings.Contains(statusLine, "200") {
		return true, nil
	}

	// read headers
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	if auth, ok := headers["WWW-Authenticate"]; ok && strings.Contains(auth, "Digest") && username != "" {
		// parse realm/nonce
		realm := extractDigestParam(auth, "realm")
		nonce := extractDigestParam(auth, "nonce")
		qop := extractDigestParam(auth, "qop")

		ha1 := md5Hash(username + ":" + realm + ":" + password)
		ha2 := md5Hash("DESCRIBE:" + getURIPath(uri))
		nc := "00000001"
		cnonce := fmt.Sprintf("%08x", time.Now().UnixNano())

		var response string
		if qop != "" {
			response = md5Hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
		} else {
			response = md5Hash(ha1 + ":" + nonce + ":" + ha2)
		}

		authHeader := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`, username, realm, nonce, getURIPath(uri), response)
		if qop != "" {
			authHeader += fmt.Sprintf(", qop=%s, nc=%s, cnonce=\"%s\"", qop, nc, cnonce)
		}

		// open new connection for authenticated DESCRIBE
		conn2, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			return false, err
		}
		defer conn2.Close()

		req2 := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 2\r\nAccept: application/sdp\r\nAuthorization: %s\r\n\r\n", uri, authHeader)
		if _, err := conn2.Write([]byte(req2)); err != nil {
			return false, err
		}

		r2 := bufio.NewReader(conn2)
		status2, err := r2.ReadString('\n')
		if err != nil {
			return false, err
		}
		status2 = strings.TrimSpace(status2)
		if strings.Contains(status2, "200") {
			return true, nil
		}
	}

	return false, nil
}

// Helper to check if namespace contains media service
func containsMedia(ns string) bool {
	return len(ns) > 0 && (
		ns == "http://www.onvif.org/ver10/media/wsdl" ||
		ns == "http://www.onvif.org/ver20/media/wsdl" ||
		// Check if contains "media" substring
		(len(ns) > 5 && (ns[len(ns)-5:] == "media" || 
		 (len(ns) > 10 && ns[len(ns)-10:] == "media/wsdl"))))
}
