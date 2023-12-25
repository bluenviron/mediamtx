package psql

func (r *Req) ExecQuery(query string) error {

	_, err := r.pool.Exec(r.ctx, query)
	if err != nil {
		return err
	}

	return nil
}
