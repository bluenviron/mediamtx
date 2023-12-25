package database

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/stretchr/testify/require"
)

func TestCreatePgxConf(t *testing.T) {

	t.Run("Not use database", func(t *testing.T) {
		confD := conf.Database{
			Use: false,
		}

		pgConf := CreatePgxConf(confD)
		require.Nil(t, pgConf)
	})

	t.Run("Use database", func(t *testing.T) {
		confD := conf.Database{
			Use:            true,
			DbUser:         "postgres",
			DbPassword:     "",
			DbAddress:      "127.0.0.1",
			DbPort:         5432,
			DbName:         "postgres",
			MaxConnections: 0,
		}

		pgConf := CreatePgxConf(confD)

		require.NotNil(t, pgConf)
	})

}
