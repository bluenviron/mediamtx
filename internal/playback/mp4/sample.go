package mp4

// Sample is a sample of a Track.
type Sample struct {
	Duration        uint32
	PTSOffset       int32
	IsNonSyncSample bool
	PayloadSize     uint32
	GetPayload      func() ([]byte, error)

	offset uint32 // filled by sortSamples
}
