package adapter

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ONVIFClient handles ONVIF protocol communication
type ONVIFClient struct {
	httpClient *http.Client
	logger     func(format string, args ...interface{})
	username   string
	password   string
}

// NewONVIFClient creates a new ONVIF client
func NewONVIFClient(logger func(format string, args ...interface{})) *ONVIFClient {
	return &ONVIFClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// NewONVIFClientWithAuth creates a new ONVIF client with stored credentials for HTTP Digest
func NewONVIFClientWithAuth(logger func(format string, args ...interface{}), username, password string) *ONVIFClient {
	return &ONVIFClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:   logger,
		username: username,
		password: password,
	}
}

// DiscoveredCamera holds info about a discovered camera
type DiscoveredCamera struct {
	Serial    string
	IP        string
	Port      int
	XAddr     string // ONVIF service address
	Model     string
	Endpoints map[string]string // service -> endpoint URL
}

// StreamProfile holds camera stream profile info
type StreamProfile struct {
	Token       string
	Name        string
	VideoWidth  int
	VideoHeight int
	VideoCodec  string
	StreamURI   string
}

// ============================================================
// WS-Discovery - Find cameras on network
// ============================================================

const wsDiscoveryProbe = `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
    xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
    xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
    xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
    <e:Header>
        <w:MessageID>uuid:%s</w:MessageID>
        <w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
        <w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
    </e:Header>
    <e:Body>
        <d:Probe>
            <d:Types>dn:NetworkVideoTransmitter</d:Types>
        </d:Probe>
    </e:Body>
</e:Envelope>`

// Alternative probe without Types filter (finds more devices)
const wsDiscoveryProbeAll = `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
    xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
    xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
    <e:Header>
        <w:MessageID>uuid:%s</w:MessageID>
        <w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
        <w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
    </e:Header>
    <e:Body>
        <d:Probe/>
    </e:Body>
</e:Envelope>`

// generateUUID creates a simple UUID v4-like string
func generateUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> (i * 4))
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Discover finds ONVIF cameras on the network
func (c *ONVIFClient) Discover(timeout time.Duration) ([]DiscoveredCamera, error) {
	c.log("Starting WS-Discovery on all interfaces...")

	// Get all local interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	// Multicast address for WS-Discovery
	multicastAddr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:3702")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	// Track unique cameras by IP
	cameraMap := make(map[string]*DiscoveredCamera)

	// Generate UUID for message ID
	msgID := generateUUID()
	
	// Try both probe types
	probes := []string{
		fmt.Sprintf(wsDiscoveryProbe, msgID),
		fmt.Sprintf(wsDiscoveryProbeAll, generateUUID()),
	}

	// Send probes from each interface
	for _, iface := range interfaces {
		// Skip loopback, down, or non-multicast interfaces
		if iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue // Skip IPv6
			}

			// Bind to this specific interface IP
			localAddr := &net.UDPAddr{IP: ip, Port: 0}
			conn, err := net.ListenUDP("udp4", localAddr)
			if err != nil {
				c.log("Failed to bind to %s: %v", ip, err)
				continue
			}

			c.log("Sending WS-Discovery from interface %s (%s)", iface.Name, ip)

			// Send both probe types
			for _, probe := range probes {
				_, err = conn.WriteToUDP([]byte(probe), multicastAddr)
				if err != nil {
					c.log("Failed to send probe from %s: %v", ip, err)
				}
			}

			// Wait for responses with timeout
			conn.SetReadDeadline(time.Now().Add(timeout))
			buf := make([]byte, 16384) // Larger buffer

			for {
				n, remoteAddr, err := conn.ReadFromUDP(buf)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						break // Normal timeout
					}
					break
				}

				response := string(buf[:n])
				c.log("WS-Discovery response from %s (len=%d)", remoteAddr.String(), n)

				// Parse response
				camera := c.parseDiscoveryResponse(response, remoteAddr.IP.String())
				if camera != nil {
					// Deduplicate by IP
					if _, exists := cameraMap[camera.IP]; !exists {
						cameraMap[camera.IP] = camera
						c.log("Found device: IP=%s, XAddr=%s", camera.IP, camera.XAddr)
					}
				}
			}

			conn.Close()
		}
	}

	// Convert map to slice
	var cameras []DiscoveredCamera
	for _, cam := range cameraMap {
		cameras = append(cameras, *cam)
	}

	c.log("Discovered %d unique device(s)", len(cameras))
	return cameras, nil
}

// parseDiscoveryResponse extracts camera info from WS-Discovery response
func (c *ONVIFClient) parseDiscoveryResponse(response, ip string) *DiscoveredCamera {
	// Extract XAddrs (ONVIF service URLs)
	xaddrRe := regexp.MustCompile(`<[^:]*:XAddrs>([^<]+)</[^:]*:XAddrs>`)
	matches := xaddrRe.FindStringSubmatch(response)
	if len(matches) < 2 {
		return nil
	}

	xaddrs := strings.Fields(matches[1])
	if len(xaddrs) == 0 {
		return nil
	}

	// Use first HTTP address
	var xaddr string
	for _, addr := range xaddrs {
		if strings.HasPrefix(addr, "http://") {
			xaddr = addr
			break
		}
	}
	if xaddr == "" {
		xaddr = xaddrs[0]
	}

	// Extract serial/endpoint reference
	serialRe := regexp.MustCompile(`<[^:]*:Address>urn:uuid:([^<]+)</[^:]*:Address>`)
	serialMatches := serialRe.FindStringSubmatch(response)
	serial := ""
	if len(serialMatches) >= 2 {
		serial = serialMatches[1]
	}

	return &DiscoveredCamera{
		Serial: serial,
		IP:     ip,
		Port:   80,
		XAddr:  xaddr,
	}
}

// ============================================================
// ONVIF Device Service
// ============================================================

// createSecurityHeader creates WS-Security header for authentication
func (c *ONVIFClient) createSecurityHeader(username, password string) string {
	// Generate nonce and timestamp
	nonce := make([]byte, 16)
	for i := range nonce {
		nonce[i] = byte(time.Now().UnixNano() >> (i * 8))
	}
	nonceBase64 := base64.StdEncoding.EncodeToString(nonce)

	created := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Calculate password digest: Base64(SHA1(nonce + created + password))
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf(`
    <Security s:mustUnderstand="1" xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <UsernameToken>
        <Username>%s</Username>
        <Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</Password>
        <Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</Nonce>
        <Created xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</Created>
      </UsernameToken>
    </Security>`, username, digest, nonceBase64, created)
}

// GetDeviceInfo gets device information via ONVIF
func (c *ONVIFClient) GetDeviceInfo(deviceURL, username, password string) (map[string]string, error) {
	soap := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>%s</s:Header>
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`, c.createSecurityHeader(username, password))

	c.log("GetDeviceInformation SOAP Request:\n%s", soap)

	resp, err := c.sendSOAP(deviceURL, soap)
	if err != nil {
		return nil, err
	}

	c.log("GetDeviceInformation SOAP Response:\n%s", resp)

	// Parse response
	info := make(map[string]string)

	// Extract fields using regex (simple parsing)
	fields := []string{"Manufacturer", "Model", "FirmwareVersion", "SerialNumber", "HardwareId"}
	for _, field := range fields {
		re := regexp.MustCompile(fmt.Sprintf(`<%s>([^<]*)</%s>`, field, field))
		matches := re.FindStringSubmatch(resp)
		if len(matches) >= 2 {
			info[field] = matches[1]
		}
	}

	return info, nil
}

// GetServices gets available ONVIF services
func (c *ONVIFClient) GetServices(deviceURL, username, password string) (map[string]string, error) {
	soap := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>%s</s:Header>
  <s:Body>
    <GetServices xmlns="http://www.onvif.org/ver10/device/wsdl">
      <IncludeCapability>false</IncludeCapability>
    </GetServices>
  </s:Body>
</s:Envelope>`, c.createSecurityHeader(username, password))

	c.log("GetServices SOAP Request:\n%s", soap)

	resp, err := c.sendSOAP(deviceURL, soap)
	if err != nil {
		return nil, err
	}

	c.log("GetServices SOAP Response:\n%s", resp)

	// Parse services
	services := make(map[string]string)

	// Extract namespace and XAddr pairs
	serviceRe := regexp.MustCompile(`<tds:Service>.*?<tds:Namespace>([^<]+)</tds:Namespace>.*?<tds:XAddr>([^<]+)</tds:XAddr>.*?</tds:Service>`)
	matches := serviceRe.FindAllStringSubmatch(resp, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			services[match[1]] = match[2]
		}
	}

	return services, nil
}

// ============================================================
// ONVIF Media Service - GetProfiles & GetStreamUri
// ============================================================

// GetProfiles gets media profiles from camera
func (c *ONVIFClient) GetProfiles(mediaURL, username, password string) ([]StreamProfile, error) {
	soap := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>%s</s:Header>
  <s:Body>
    <GetProfiles xmlns="http://www.onvif.org/ver10/media/wsdl"/>
  </s:Body>
</s:Envelope>`, c.createSecurityHeader(username, password))

	c.log("GetProfiles SOAP Request:\n%s", soap)

	resp, err := c.sendSOAP(mediaURL, soap)
	if err != nil {
		return nil, err
	}

	c.log("GetProfiles SOAP Response:\n%s", resp)

	// Parse profiles
	var profiles []StreamProfile

	// Extract profile tokens and names
	profileRe := regexp.MustCompile(`<trt:Profiles[^>]*token="([^"]+)"[^>]*>.*?<tt:Name>([^<]*)</tt:Name>`)
	matches := profileRe.FindAllStringSubmatch(resp, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			profiles = append(profiles, StreamProfile{
				Token: match[1],
				Name:  match[2],
			})
		}
	}

	// If regex didn't work, try alternative pattern
	if len(profiles) == 0 {
		altRe := regexp.MustCompile(`token="([^"]+)"`)
		matches := altRe.FindAllStringSubmatch(resp, -1)
		for i, match := range matches {
			if len(match) >= 2 && i < 5 { // Limit to first 5
				profiles = append(profiles, StreamProfile{
					Token: match[1],
					Name:  fmt.Sprintf("Profile_%d", i+1),
				})
			}
		}
	}

	c.log("Found %d profiles", len(profiles))
	return profiles, nil
}

// GetStreamURI gets RTSP stream URI for a profile
func (c *ONVIFClient) GetStreamURI(mediaURL, username, password, profileToken string) (string, error) {
	soap := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>%s</s:Header>
  <s:Body>
    <GetStreamUri xmlns="http://www.onvif.org/ver10/media/wsdl">
      <StreamSetup>
        <Stream xmlns="http://www.onvif.org/ver10/schema">RTP-Unicast</Stream>
        <Transport xmlns="http://www.onvif.org/ver10/schema">
          <Protocol>RTSP</Protocol>
        </Transport>
      </StreamSetup>
      <ProfileToken>%s</ProfileToken>
    </GetStreamUri>
  </s:Body>
</s:Envelope>`, c.createSecurityHeader(username, password), profileToken)

	c.log("GetStreamUri SOAP Request:\n%s", soap)

	resp, err := c.sendSOAP(mediaURL, soap)
	if err != nil {
		return "", err
	}

	c.log("GetStreamUri SOAP Response:\n%s", resp)

	// Extract URI
	uriRe := regexp.MustCompile(`<tt:Uri>([^<]+)</tt:Uri>`)
	matches := uriRe.FindStringSubmatch(resp)
	if len(matches) < 2 {
		// Try alternative pattern
		uriRe = regexp.MustCompile(`<[^:]*Uri>([^<]+)</[^:]*Uri>`)
		matches = uriRe.FindStringSubmatch(resp)
		if len(matches) < 2 {
			return "", fmt.Errorf("URI not found in response")
		}
	}

	uri := matches[1]
	
	// Decode XML/HTML entities
	uri = strings.ReplaceAll(uri, "&amp;", "&")
	uri = strings.ReplaceAll(uri, "&lt;", "<")
	uri = strings.ReplaceAll(uri, "&gt;", ">")
	uri = strings.ReplaceAll(uri, "&quot;", "\"")
	uri = strings.ReplaceAll(uri, "&apos;", "'")

	// Add credentials to URI if not present
	if !strings.Contains(uri, "@") && username != "" {
		uri = strings.Replace(uri, "rtsp://", fmt.Sprintf("rtsp://%s:%s@", username, password), 1)
	}

	c.log("Stream URI: %s", uri)
	return uri, nil
}

// ============================================================
// High-level helper: Get RTSP URL from camera
// ============================================================

// GetRTSPURL discovers camera and gets RTSP URL using credentials
func (c *ONVIFClient) GetRTSPURL(cameraIP string, port int, username, password string) (string, error) {
	// Build device service URL
	deviceURL := fmt.Sprintf("http://%s:%d/onvif/device_service", cameraIP, port)

	c.log("Connecting to ONVIF device at %s", deviceURL)

	// 1. Get device info (optional, for logging)
	info, err := c.GetDeviceInfo(deviceURL, username, password)
	if err != nil {
		c.log("Warning: Could not get device info: %v", err)
	} else {
		c.log("Device: %s %s (Serial: %s)", info["Manufacturer"], info["Model"], info["SerialNumber"])
	}

	// 2. Try to get services to find media service URL
	mediaURL := fmt.Sprintf("http://%s:%d/onvif/media", cameraIP, port)
	services, err := c.GetServices(deviceURL, username, password)
	if err == nil {
		// Look for media service
		for ns, url := range services {
			if strings.Contains(ns, "media") {
				mediaURL = url
				break
			}
		}
	}

	c.log("Media service URL: %s", mediaURL)

	// 3. Get profiles
	profiles, err := c.GetProfiles(mediaURL, username, password)
	if err != nil {
		return "", fmt.Errorf("failed to get profiles: %w", err)
	}

	if len(profiles) == 0 {
		return "", fmt.Errorf("no profiles found")
	}

	// Use first profile (usually main stream)
	profile := profiles[0]
	c.log("Using profile: %s (%s)", profile.Token, profile.Name)

	// 4. Get stream URI
	streamURI, err := c.GetStreamURI(mediaURL, username, password, profile.Token)
	if err != nil {
		return "", fmt.Errorf("failed to get stream URI: %w", err)
	}

	return streamURI, nil
}

// ============================================================
// Helper methods
// ============================================================

// sendSOAP sends a SOAP request and returns the response
func (c *ONVIFClient) sendSOAP(url, body string) (string, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if we got 401 Unauthorized - need HTTP Digest Auth
	if resp.StatusCode == http.StatusUnauthorized {
		// Try HTTP Digest authentication
		authHeader := resp.Header.Get("WWW-Authenticate")
		if strings.Contains(authHeader, "Digest") && c.username != "" {
			c.log("Trying HTTP Digest authentication...")
			return c.sendSOAPWithDigest(url, body, authHeader)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return string(respBody), fmt.Errorf("SOAP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// sendSOAPWithDigest sends a SOAP request with HTTP Digest authentication
func (c *ONVIFClient) sendSOAPWithDigest(url, body, authHeader string) (string, error) {
	// Parse WWW-Authenticate header
	realm := extractDigestParam(authHeader, "realm")
	nonce := extractDigestParam(authHeader, "nonce")
	qop := extractDigestParam(authHeader, "qop")

	// Calculate digest response
	ha1 := md5Hash(c.username + ":" + realm + ":" + c.password)
	ha2 := md5Hash("POST:" + getURIPath(url))

	nc := "00000001"
	cnonce := fmt.Sprintf("%08x", time.Now().UnixNano())

	var response string
	if qop != "" {
		response = md5Hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		response = md5Hash(ha1 + ":" + nonce + ":" + ha2)
	}

	// Build Authorization header
	auth := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		c.username, realm, nonce, getURIPath(url), response)
	if qop != "" {
		auth += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("Authorization", auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return string(respBody), fmt.Errorf("SOAP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// Helper to extract param from Digest auth header
func extractDigestParam(header, param string) string {
	re := regexp.MustCompile(param + `="?([^",]+)"?`)
	matches := re.FindStringSubmatch(header)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// Helper to get URI path from full URL
func getURIPath(fullURL string) string {
	re := regexp.MustCompile(`https?://[^/]+(.*)`)
	matches := re.FindStringSubmatch(fullURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	return "/"
}

// MD5 hash helper
func md5Hash(data string) string {
	h := md5.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *ONVIFClient) log(format string, args ...interface{}) {
	if c.logger != nil {
		c.logger("[ONVIF] "+format, args...)
	}
}

// ============================================================
// SOAP Response Structures (for XML parsing if needed)
// ============================================================

// Envelope represents SOAP envelope
type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    Body     `xml:"Body"`
}

// Body represents SOAP body
type Body struct {
	GetProfilesResponse      *GetProfilesResponse      `xml:"GetProfilesResponse"`
	GetStreamUriResponse     *GetStreamUriResponse     `xml:"GetStreamUriResponse"`
	GetDeviceInfoResponse    *GetDeviceInfoResponse    `xml:"GetDeviceInformationResponse"`
}

// GetProfilesResponse holds GetProfiles response
type GetProfilesResponse struct {
	Profiles []Profile `xml:"Profiles"`
}

// Profile represents a media profile
type Profile struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"Name"`
}

// GetStreamUriResponse holds GetStreamUri response
type GetStreamUriResponse struct {
	MediaUri MediaUri `xml:"MediaUri"`
}

// MediaUri holds stream URI info
type MediaUri struct {
	Uri string `xml:"Uri"`
}

// GetDeviceInfoResponse holds device info response
type GetDeviceInfoResponse struct {
	Manufacturer    string `xml:"Manufacturer"`
	Model           string `xml:"Model"`
	FirmwareVersion string `xml:"FirmwareVersion"`
	SerialNumber    string `xml:"SerialNumber"`
	HardwareId      string `xml:"HardwareId"`
}
