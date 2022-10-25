package rpicamera

// Params is a set of camera parameters.
type Params struct {
	CameraID   int
	Width      int
	Height     int
	HFlip      bool
	VFlip      bool
	Brightness float64
	Contrast   float64
	Saturation float64
	Sharpness  float64
	Exposure   string
	AWB        string
	Denoise    string
	Shutter    int
	Metering   string
	Gain       float64
	EV         float64
	ROI        string
	TuningFile string
	Mode       string
	FPS        int
	IDRPeriod  int
	Bitrate    int
	Profile    string
	Level      string
}
