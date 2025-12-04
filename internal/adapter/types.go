package adapter

import (
	"time"
)

// SignalingType represents the type of signaling message
type SignalingType string

const (
	SignalingTypeRequest      SignalingType = "request"
	SignalingTypeOffer        SignalingType = "offer"
	SignalingTypeAnswer       SignalingType = "answer"
	SignalingTypeDeny         SignalingType = "deny"
	SignalingTypeCCU          SignalingType = "ccu"
	SignalingTypeIceCandidate SignalingType = "ice-candidate"
)

// ResultCode represents the result code in responses
type ResultCode int

const (
	ResultSuccess       ResultCode = 100
	ResultFail          ResultCode = 103
	ResultStreamSuccess ResultCode = 0
)

// Result represents the result object in messages
type Result struct {
	Ret     ResultCode `json:"Ret"`
	Message string     `json:"Message"`
}

// SignalingMessage represents the base signaling message structure
type SignalingMessage struct {
	Method      string                 `json:"Method"`
	MessageType string                 `json:"MessageType"`
	Serial      string                 `json:"Serial"`
	Data        map[string]interface{} `json:"Data"`
	Timestamp   int64                  `json:"Timestamp"`
	Result      *Result                `json:"Result,omitempty"`
}

// SignalingRequest represents a signaling request from client
type SignalingRequest struct {
	Method      string       `json:"Method"`
	MessageType string       `json:"MessageType"`
	Serial      string       `json:"Serial"`
	Data        RequestData  `json:"Data"`
	Timestamp   int64        `json:"Timestamp"`
}

// RequestData represents the data in a signaling request
type RequestData struct {
	Type     SignalingType `json:"Type"`
	ClientID string        `json:"ClientId"`
	SDP      string        `json:"Sdp,omitempty"`
}

// SignalingResponse represents a signaling response to client
type SignalingResponse struct {
	Method      string       `json:"Method"`
	MessageType string       `json:"MessageType"`
	Serial      string       `json:"Serial"`
	Data        ResponseData `json:"Data"`
	Timestamp   int64        `json:"Timestamp"`
	Result      Result       `json:"Result"`
}

// ResponseData represents the data in a signaling response
type ResponseData struct {
	Type                SignalingType `json:"Type"`
	ClientID            string        `json:"ClientId,omitempty"`
	SDP                 string        `json:"Sdp,omitempty"`
	IceServers          []string      `json:"IceServers,omitempty"`
	ClientMax           int           `json:"ClientMax,omitempty"`
	CurrentClientsTotal int           `json:"CurrentClientsTotal,omitempty"`
}

// DenyResponse represents a deny response when max clients reached
type DenyResponse struct {
	Method      string   `json:"Method"`
	MessageType string   `json:"MessageType"`
	Serial      string   `json:"Serial"`
	Data        DenyData `json:"Data"`
	Timestamp   int64    `json:"Timestamp"`
	Result      Result   `json:"Result"`
}

// DenyData represents data in a deny response
type DenyData struct {
	Type                SignalingType `json:"Type"`
	ClientID            string        `json:"ClientId"`
	ClientMax           int           `json:"ClientMax"`
	CurrentClientsTotal int           `json:"CurrentClientsTotal"`
}

// OfferResponse represents an offer response from camera/adapter
type OfferResponse struct {
	Method      string    `json:"Method"`
	MessageType string    `json:"MessageType"`
	Serial      string    `json:"Serial"`
	Data        OfferData `json:"Data"`
	Timestamp   int64     `json:"Timestamp"`
	Result      Result    `json:"Result"`
}

// OfferData represents data in an offer response
type OfferData struct {
	Type       SignalingType `json:"Type"`
	ClientID   string        `json:"ClientId"`
	SDP        string        `json:"Sdp"`
	IceServers []string      `json:"IceServers,omitempty"`
}

// AnswerRequest represents an answer from client
type AnswerRequest struct {
	Method      string     `json:"Method"`
	MessageType string     `json:"MessageType"`
	Serial      string     `json:"Serial"`
	Data        AnswerData `json:"Data"`
	Timestamp   int64      `json:"Timestamp"`
	Result      *Result    `json:"Result,omitempty"`
}

// AnswerData represents data in an answer request
type AnswerData struct {
	Type     SignalingType `json:"Type"`
	ClientID string        `json:"ClientId"`
	SDP      string        `json:"Sdp"`
}

// CCUResponse represents a CCU (Concurrent Client Update) response
type CCUResponse struct {
	Method      string  `json:"Method"`
	MessageType string  `json:"MessageType"`
	Serial      string  `json:"Serial"`
	Data        CCUData `json:"Data"`
	Timestamp   int64   `json:"Timestamp"`
	Result      Result  `json:"Result"`
}

// CCUData represents data in a CCU response
type CCUData struct {
	Type                SignalingType `json:"Type"`
	CurrentClientsTotal int           `json:"CurrentClientsTotal"`
}

// IceCandidateMessage represents an ICE candidate exchange message
type IceCandidateMessage struct {
	Method      string           `json:"Method"`
	MessageType string           `json:"MessageType"`
	Serial      string           `json:"Serial"`
	Data        IceCandidateData `json:"Data"`
	Timestamp   int64            `json:"Timestamp"`
}

// IceCandidateData represents data in an ICE candidate message
type IceCandidateData struct {
	Type      SignalingType `json:"Type"`
	ClientID  string        `json:"ClientId"`
	Candidate IceCandidate  `json:"Candidate"`
}

// IceCandidate represents a single ICE candidate
type IceCandidate struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
}

// CredentialMessage represents camera credentials from client
type CredentialMessage struct {
	Method      string         `json:"Method"`
	MessageType string         `json:"MessageType"`
	Serial      string         `json:"Serial"`
	Data        CredentialData `json:"Data"`
	Timestamp   int64          `json:"Timestamp"`
}

// CredentialData represents camera credentials
type CredentialData struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
	IP       string `json:"IP,omitempty"` // Camera IP address (optional, can be discovered via ONVIF)
}

// DataChannelCommand represents commands sent via data channel
type DataChannelCommand string

const (
	DataChannelCommandStream      DataChannelCommand = "Stream"
	DataChannelCommandOnvifStatus DataChannelCommand = "OnvifStatus"
)

// DataChannelRequest represents a request via data channel
type DataChannelRequest struct {
	ID      string             `json:"Id"`
	Command DataChannelCommand `json:"Command"`
	Type    string             `json:"Type"`
	Content interface{}        `json:"Content"`
}

// StreamContent represents content in a stream request
type StreamContent struct {
	ChannelMask    int64 `json:"ChannelMask"`
	ResolutionMask int64 `json:"ResolutionMask"`
}

// DataChannelResponse represents a response via data channel
type DataChannelResponse struct {
	ID      string             `json:"Id"`
	Command DataChannelCommand `json:"Command"`
	Type    string             `json:"Type"`
	Content interface{}        `json:"Content"`
	Result  Result             `json:"Result"`
}

// OnvifStatus represents the status of an ONVIF camera channel
type OnvifStatus struct {
	StreamID int    `json:"Stream id"`
	IP       string `json:"IP"`
	User     string `json:"User"`
	Pass     string `json:"Pass"`
	FullHD   string `json:"FullHD"`
	HD       string `json:"HD"`
}

// OnvifStatusContent represents the content of an ONVIF status response
type OnvifStatusContent struct {
	OnvifStatus []OnvifStatus `json:"OnvifStatus"`
}

// NewSignalingRequest creates a new signaling request
func NewSignalingRequest(serial, clientID string, sigType SignalingType) *SignalingRequest {
	return &SignalingRequest{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: RequestData{
			Type:     sigType,
			ClientID: clientID,
		},
		Timestamp: time.Now().Unix(),
	}
}

// NewOfferResponse creates a new offer response
func NewOfferResponse(serial, clientID, sdp string, iceServers []string) *OfferResponse {
	return &OfferResponse{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: OfferData{
			Type:       SignalingTypeOffer,
			ClientID:   clientID,
			SDP:        sdp,
			IceServers: iceServers,
		},
		Timestamp: time.Now().Unix(),
		Result: Result{
			Ret:     ResultSuccess,
			Message: "Success",
		},
	}
}

// NewAnswerResponse creates a new answer acknowledgment response
func NewAnswerResponse(serial, clientID string) *SignalingResponse {
	return &SignalingResponse{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: ResponseData{
			Type:     SignalingTypeAnswer,
			ClientID: clientID,
		},
		Timestamp: time.Now().Unix(),
		Result: Result{
			Ret:     ResultSuccess,
			Message: "Success",
		},
	}
}

// NewDenyResponse creates a new deny response
func NewDenyResponse(serial, clientID string, maxClients, currentClients int) *DenyResponse {
	return &DenyResponse{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: DenyData{
			Type:                SignalingTypeDeny,
			ClientID:            clientID,
			ClientMax:           maxClients,
			CurrentClientsTotal: currentClients,
		},
		Timestamp: time.Now().Unix(),
		Result: Result{
			Ret:     ResultFail,
			Message: "Fail",
		},
	}
}

// NewCCUResponse creates a new CCU response
func NewCCUResponse(serial string, currentClients int) *CCUResponse {
	return &CCUResponse{
		Method:      "ACT",
		MessageType: "Signaling",
		Serial:      serial,
		Data: CCUData{
			Type:                SignalingTypeCCU,
			CurrentClientsTotal: currentClients,
		},
		Timestamp: time.Now().Unix(),
		Result: Result{
			Ret:     ResultSuccess,
			Message: "Success",
		},
	}
}
