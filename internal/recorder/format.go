package recorder

type format interface {
	initialize()
	close()
}
