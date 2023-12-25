package psql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Req struct {
	ctx  context.Context
	pool *pgxpool.Pool
}

func NewReq(ctx context.Context, pool *pgxpool.Pool) *Req {
	return &Req{
		ctx:  ctx,
		pool: pool,
	}
}
