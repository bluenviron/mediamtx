package defs

// SRTLAGroupInfo contains information about an SRTLA group for correlation purposes.
type SRTLAGroupInfo struct {
	Path           string
	ConnsActive    int
	BytesReceived  uint64
	BytesForwarded uint64
}

// SRTLALinker allows the SRT server to correlate connections with SRTLA groups.
type SRTLALinker interface {
	// SetGroupPath sets the stream path on the SRTLA group identified by the SRT connection's remote address.
	SetGroupPath(srtConnAddr string, path string)
	// CloseGroupByAddr closes the SRTLA group associated with the given SRT connection address.
	CloseGroupByAddr(srtConnAddr string)
}

// APISRTLAServer contains methods used by the Metrics server.
type APISRTLAServer interface {
	APISRTLAGroupsList() []APISRTLAGroup
}

// APISRTLAGroup is an SRTLA group entry for metrics/API.
type APISRTLAGroup struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	ConnsActive    int    `json:"connsActive"`
	BytesReceived  uint64 `json:"bytesReceived"`
	BytesForwarded uint64 `json:"bytesForwarded"`
}
