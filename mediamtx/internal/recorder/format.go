package recorder

type format interface {
	initialize() bool
	close()
}
