package conf

// WebRTCICEServer is a WebRTC ICE Server.
type WebRTCICEServer struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}
