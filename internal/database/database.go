package database

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreatePgxConf(cfg conf.Database) *pgxpool.Config {

	if cfg.Use {
		conf, _ := pgxpool.ParseConfig("")
		conf.ConnConfig.User = cfg.DbUser
		conf.ConnConfig.Password = cfg.DbPassword
		conf.ConnConfig.Host = cfg.DbAddress
		conf.ConnConfig.Port = uint16(cfg.DbPort)
		conf.ConnConfig.Database = cfg.DbName
		conf.MaxConns = int32(cfg.MaxConnections)
		conf.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return conf
	}

	return nil

}

func CreateDbPool(ctx context.Context, conf *pgxpool.Config) (*pgxpool.Pool, error) {

	if conf == nil {
		return nil, nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, err
	}

	_, err = pool.Exec(ctx, "SELECT '1'")
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func ClosePool(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}
