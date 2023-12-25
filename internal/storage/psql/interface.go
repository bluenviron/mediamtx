package psql

type Requests interface {
	ExecQuery(query string) error
}
