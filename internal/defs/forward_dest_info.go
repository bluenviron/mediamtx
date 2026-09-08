package defs

// ForwardDestInfo contains runtime information of a forward destination.
type ForwardDestInfo struct {
	OutboundBytes uint64
	TypeSpecific  APIForwardDestTypeSpecific
}
