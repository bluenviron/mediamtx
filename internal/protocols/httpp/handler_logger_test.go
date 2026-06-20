package httpp

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDumpRequestLimitedRedactsSensitiveHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", strings.NewReader("request body"))
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-cookie")
	req.Header.Set("Proxy-Authorization", "Bearer proxy-secret")
	req.Header.Set("X-Api-Key", "secret-api-key")
	req.Header.Set("X-Auth-Token", "secret-auth-token")

	dump, err := dumpRequestLimited(req)
	require.NoError(t, err)

	dumpStr := string(dump)
	require.Contains(t, dumpStr, "Authorization: <redacted>")
	require.Contains(t, dumpStr, "Cookie: <redacted>")
	require.Contains(t, dumpStr, "Proxy-Authorization: <redacted>")
	require.Contains(t, dumpStr, "X-Api-Key: <redacted>")
	require.Contains(t, dumpStr, "X-Auth-Token: <redacted>")
	require.NotContains(t, dumpStr, "secret-token")
	require.NotContains(t, dumpStr, "secret-cookie")
	require.NotContains(t, dumpStr, "proxy-secret")
	require.NotContains(t, dumpStr, "secret-api-key")
	require.NotContains(t, dumpStr, "secret-auth-token")
	require.Contains(t, dumpStr, "request body")

	require.Equal(t, "Bearer secret-token", req.Header.Get("Authorization"))

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "request body", string(body))
}
