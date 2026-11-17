package conf_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func TestCredential(t *testing.T) {
	t.Run("UnmarshalJSON", func(t *testing.T) {
		expectedCred := conf.Credential("password")
		jsonData := []byte(`"password"`)
		var actualCred conf.Credential
		err := actualCred.UnmarshalJSON(jsonData)
		assert.NoError(t, err)
		assert.Equal(t, expectedCred, actualCred)
	})

	t.Run("UnmarshalEnv", func(t *testing.T) {
		cred := conf.Credential("")
		err := cred.UnmarshalEnv("", "password")
		assert.NoError(t, err)
		assert.Equal(t, conf.Credential("password"), cred)
	})

	t.Run("IsSha256", func(t *testing.T) {
		cred := conf.Credential("")
		assert.False(t, cred.IsSha256())
		assert.False(t, cred.IsHashed())

		cred = "sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo="
		assert.True(t, cred.IsSha256())
		assert.True(t, cred.IsHashed())

		cred = "argon2:$argon2id$v=19$m=65536,t=1," +
			"p=4$WXJGqwIB2qd+pRmxMOw9Dg$X4gvR0ZB2DtQoN8vOnJPR2SeFdUhH9TyVzfV98sfWeE"
		assert.False(t, cred.IsSha256())
		assert.True(t, cred.IsHashed())
	})

	t.Run("IsArgon2", func(t *testing.T) {
		cred := conf.Credential("")
		assert.False(t, cred.IsArgon2())
		assert.False(t, cred.IsHashed())

		cred = "sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo="
		assert.False(t, cred.IsArgon2())
		assert.True(t, cred.IsHashed())

		cred = "argon2:$argon2id$v=19$m=65536,t=1," +
			"p=4$WXJGqwIB2qd+pRmxMOw9Dg$X4gvR0ZB2DtQoN8vOnJPR2SeFdUhH9TyVzfV98sfWeE"
		assert.True(t, cred.IsArgon2())
		assert.True(t, cred.IsHashed())
	})

	t.Run("Check-plain", func(t *testing.T) {
		cred := conf.Credential("password")
		assert.True(t, cred.Check("password"))
		assert.False(t, cred.Check("wrongpassword"))
	})

	t.Run("Check-sha256", func(t *testing.T) {
		cred := conf.Credential("password")
		assert.True(t, cred.Check("password"))
		assert.False(t, cred.Check("wrongpassword"))
	})

	t.Run("Check-sha256", func(t *testing.T) {
		cred := conf.Credential("sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=")
		assert.True(t, cred.Check("testuser"))
		assert.False(t, cred.Check("notestuser"))
	})

	t.Run("Check-argon2", func(t *testing.T) {
		cred := conf.Credential("argon2:$argon2id$v=19$m=4096,t=3," +
			"p=1$MTIzNDU2Nzg$Ux/LWeTgJQPyfMMJo1myR64+o8rALHoPmlE1i/TR+58")
		assert.True(t, cred.Check("testuser"))
		assert.False(t, cred.Check("notestuser"))
	})

	t.Run("validate", func(t *testing.T) {
		tests := []struct {
			name    string
			cred    conf.Credential
			wantErr bool
		}{
			{
				name:    "Empty credential",
				cred:    conf.Credential(""),
				wantErr: false,
			},
			{
				name:    "Valid plain credential",
				cred:    conf.Credential("validPlain123"),
				wantErr: false,
			},
			{
				name:    "Invalid plain credential",
				cred:    conf.Credential("invalid/Plain"),
				wantErr: true,
			},
			{
				name:    "Valid sha256 credential",
				cred:    conf.Credential("sha256:validBase64EncodedHash=="),
				wantErr: false,
			},
			{
				name:    "Invalid sha256 credential",
				cred:    conf.Credential("sha256:inval*idBase64"),
				wantErr: true,
			},
			{
				name: "Valid Argon2 credential",
				cred: conf.Credential("argon2:$argon2id$v=19$m=4096," +
					"t=3,p=1$MTIzNDU2Nzg$zarsL19s86GzUWlAkvwt4gJBFuU/A9CVuCjNI4fksow"),
				wantErr: false,
			},
			{
				name:    "Invalid Argon2 credential",
				cred:    conf.Credential("argon2:invalid"),
				wantErr: true,
			},
			{
				name: "Invalid Argon2 credential",
				// testing argon2d errors, because it's not supported
				cred: conf.Credential("$argon2d$v=19$m=4096,t=3," +
					"p=1$MTIzNDU2Nzg$Xqyd4R7LzXvvAEHaVU12+Nzf5OkHoYcwIEIIYJUDpz0"),
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := json.Unmarshal([]byte(strconv.Quote(string(tt.cred))), new(conf.Credential))
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}
