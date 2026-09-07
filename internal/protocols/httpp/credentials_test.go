package httpp_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

func TestCredentials(t *testing.T) {
	t.Run("user and pass in basic", func(t *testing.T) {
		h := &http.Request{
			URL: &url.URL{},
			Header: http.Header{
				"Authorization": []string{
					"Basic bXl1c2VyOm15cGFzcw==",
				},
			},
		}

		c := httpp.Credentials(h)

		require.Equal(t, &auth.Credentials{
			User: "myuser",
			Pass: "mypass",
		}, c)
	})

	t.Run("user and pass in bearer", func(t *testing.T) {
		h := &http.Request{
			URL: &url.URL{},
			Header: http.Header{
				"Authorization": []string{
					"Bearer myuser:mypass",
				},
			},
		}

		c := httpp.Credentials(h)

		require.Equal(t, &auth.Credentials{
			User: "myuser",
			Pass: "mypass",
		}, c)
	})

	t.Run("token in bearer", func(t *testing.T) {
		h := &http.Request{
			URL: &url.URL{},
			Header: http.Header{
				"Authorization": []string{
					"Bearer testing123",
				},
			},
		}

		c := httpp.Credentials(h)

		require.Equal(t, &auth.Credentials{
			Token: "testing123",
		}, c)
	})

	t.Run("user and pass and token", func(t *testing.T) {
		h := &http.Request{
			URL: &url.URL{},
			Header: http.Header{
				"Authorization": []string{
					"Basic bXl1c2VyOm15cGFzcw==",
					"Bearer testing123",
				},
			},
		}

		c := httpp.Credentials(h)

		require.Equal(t, &auth.Credentials{
			Token: "testing123",
		}, c)
	})
}
