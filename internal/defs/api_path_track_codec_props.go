package defs

// APIPathTrackCodecProps contains codec-specific properties.
type APIPathTrackCodecProps interface {
	apiPathTrackCodecProps()
}

// APIPathTrackCodecPropsAV1 contains codec-specific properties of AV1.
type APIPathTrackCodecPropsAV1 struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	Profile int `json:"profile"`
	Level   int `json:"level"`
	Tier    int `json:"tier"`
}

func (*APIPathTrackCodecPropsAV1) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsVP9 contains codec-specific properties of VP9.
type APIPathTrackCodecPropsVP9 struct {
	Profile int `json:"profile"`
}

func (*APIPathTrackCodecPropsVP9) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsH265 contains codec-specific properties of H265.
type APIPathTrackCodecPropsH265 struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Profile string `json:"profile"`
	Level   string `json:"level"`
}

func (*APIPathTrackCodecPropsH265) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsH264 contains codec-specific properties of H264.
type APIPathTrackCodecPropsH264 struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Profile string `json:"profile"`
	Level   string `json:"level"`
}

func (*APIPathTrackCodecPropsH264) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsOpus contains codec-specific properties of Opus.
type APIPathTrackCodecPropsOpus struct {
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsOpus) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsMPEG4Audio contains codec-specific properties of MPEG4Audio.
type APIPathTrackCodecPropsMPEG4Audio struct {
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsMPEG4Audio) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsAC3 contains codec-specific properties of AC3.
type APIPathTrackCodecPropsAC3 struct {
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsAC3) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsG711 contains codec-specific properties of G711.
type APIPathTrackCodecPropsG711 struct {
	MULaw        bool `json:"muLaw"`
	SampleRate   int  `json:"sampleRate"`
	ChannelCount int  `json:"channelCount"`
}

func (*APIPathTrackCodecPropsG711) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsLPCM contains codec-specific properties of LPCM.
type APIPathTrackCodecPropsLPCM struct {
	BitDepth     int `json:"bitDepth"`
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsLPCM) apiPathTrackCodecProps() {}
