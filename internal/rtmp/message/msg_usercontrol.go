package message

// user control types.
const (
	UserControlTypeStreamBegin      = 0
	UserControlTypeStreamEOF        = 1
	UserControlTypeStreamDry        = 2
	UserControlTypeSetBufferLength  = 3
	UserControlTypeStreamIsRecorded = 4
	UserControlTypePingRequest      = 6
	UserControlTypePingResponse     = 7
)
