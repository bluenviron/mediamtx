package record

type format interface {
	initialize()
	close()
}
