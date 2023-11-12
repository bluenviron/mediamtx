package webrtc

import (
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var videoCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeAV1,
			ClockRate: 90000,
		},
		PayloadType: 96,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
		PayloadType: 97,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=1",
		},
		PayloadType: 98,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 99,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 100,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 101,
	},
}

var audioCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
		},
		PayloadType: 111,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeG722,
			ClockRate: 8000,
		},
		PayloadType: 9,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	},
}

// APIConf is the configuration passed to NewAPI().
type APIConf struct {
	ICEUDPMux             ice.UDPMux
	ICETCPMux             ice.TCPMux
	LocalRandomUDP        bool
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
}

// NewAPI allocates a webrtc API.
func NewAPI(cnf APIConf) (*webrtc.API, error) {
	settingsEngine := webrtc.SettingEngine{}

	settingsEngine.SetInterfaceFilter(func(iface string) bool {
		return cnf.IPsFromInterfaces && (len(cnf.IPsFromInterfacesList) == 0 ||
			stringInSlice(iface, cnf.IPsFromInterfacesList))
	})

	settingsEngine.SetAdditionalHosts(cnf.AdditionalHosts)

	var networkTypes []webrtc.NetworkType

	// always enable UDP in order to support STUN/TURN
	networkTypes = append(networkTypes, webrtc.NetworkTypeUDP4)

	if cnf.ICEUDPMux != nil {
		settingsEngine.SetICEUDPMux(cnf.ICEUDPMux)
	}

	if cnf.ICETCPMux != nil {
		settingsEngine.SetICETCPMux(cnf.ICETCPMux)
		networkTypes = append(networkTypes, webrtc.NetworkTypeTCP4)
	}

	if cnf.LocalRandomUDP {
		settingsEngine.SetICEUDPRandom(true)
	}

	settingsEngine.SetNetworkTypes(networkTypes)

	mediaEngine := &webrtc.MediaEngine{}

	for _, codec := range videoCodecs {
		err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeVideo)
		if err != nil {
			return nil, err
		}
	}

	for _, codec := range audioCodecs {
		err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeAudio)
		if err != nil {
			return nil, err
		}
	}

	interceptorRegistry := &interceptor.Registry{}

	err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry)
	if err != nil {
		return nil, err
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(settingsEngine),
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry)), nil
}
