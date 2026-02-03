# Authentication

## Overview

_MediaMTX_ can be configured to ask clients for credentials, either in the form of username/password or a string-based token. These credentials are then validated through a chosen method.

## Credential validation

Credentials can be validated through one of these methods:

- Internal database: credentials are stored in the configuration file
- External HTTP server: an external HTTP URL is contacted to perform authentication
- External JWT provider: an external identity server provides signed tokens that are then verified by the server

### Internal database

The internal authentication method is the default one. Users are stored inside the configuration file, in this format:

```yml
authInternalUsers:
  # Username. 'any' means any user, including anonymous ones.
  - user: any
    # Password. Not used in case of 'any' user.
    pass:
    # IPs or networks allowed to use this user. An empty list means any IP.
    ips: []
    # Permissions.
    permissions:
      # Available actions are: publish, read, playback, api, metrics, pprof.
      - action: publish
        # Paths can be set to further restrict access to a specific path.
        # An empty path means any path.
        # Regular expressions can be used by using a tilde as prefix.
        path:
      - action: read
        path:
      - action: playback
        path:
```

Only clients that provide a valid username and password will be able to perform a certain action.

If storing plain credentials in the configuration file is a security problem, username and passwords can be stored as hashed strings. The Argon2 and SHA256 hashing algorithms are supported. To use Argon2, the string must be hashed using Argon2id (recommended) or Argon2i:

```
echo -n "mypass" | argon2 saltItWithSalt -id -l 32 -e
```

Then stored with the `argon2:` prefix:

```yml
authInternalUsers:
  - user: argon2:$argon2id$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$OGGO0eCMN0ievb4YGSzvS/H+Vajx1pcbUmtLp2tRqRU
    pass: argon2:$argon2i$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$oct3kOiFywTdDdt19kT07hdvmsPTvt9zxAUho2DLqZw
    permissions:
      - action: publish
```

To use SHA256, the string must be hashed with SHA256 and encoded with base64:

```
echo -n "mypass" | openssl dgst -binary -sha256 | openssl base64
```

Then stored with the `sha256:` prefix:

```yml
authInternalUsers:
  - user: sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo=
    pass: sha256:BdSWkrdV+ZxFBLUQQY7+7uv9RmiSVA8nrPmjGjJtZQQ=
    permissions:
      - action: publish
```

**WARNING**: enable encryption or use a VPN to ensure that no one is intercepting the credentials in transit.

### External HTTP server

Authentication can be delegated to an external HTTP server:

```yml
authMethod: http
authHTTPAddress: http://myauthserver/auth
```

Each time a user needs to be authenticated, the specified URL will be requested with the POST method and this payload:

```json
{
  "user": "user",
  "password": "password",
  "token": "token",
  "ip": "ip",
  "action": "publish|read|playback|api|metrics|pprof",
  "path": "path",
  "protocol": "rtsp|rtmp|hls|webrtc|srt",
  "id": "id",
  "query": "query"
}
```

If the URL returns a status code that begins with `20` (i.e. `200`), authentication is successful, otherwise it fails. Be aware that it's perfectly normal for the authentication server to receive requests with empty users and passwords, i.e.:

```json
{
  "user": "",
  "password": ""
}
```

This happens because RTSP clients don't provide credentials until they are asked to. In order to receive the credentials, the authentication server must reply with status code `401`, then the client will send credentials.

Some actions can be excluded from the process:

```yml
# Actions to exclude from HTTP-based authentication.
# Format is the same as the one of user permissions.
authHTTPExclude:
  - action: api
  - action: metrics
  - action: pprof
```

If the authentication server uses HTTPS and has a self-signed or invalid TLS certificate, you can provide the fingerprint of the certificate to validate it anyway:

```yml
authMethod: http
authHTTPAddress: https://myauthserver/auth
authHTTPFingerprint: 33949e05fffb5ff3e8aa16f8213a6251b4d9363804ba53233c4da9a46d6f2739
```

The fingerprint can be obtained with:

```sh
openssl s_client -connect myauthserver:443 </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
```

### External JWT provider

Authentication can be delegated to an external identity server, that is capable of generating JWTs and provides a JWKS endpoint. With respect to the HTTP-based method, this has the advantage that the external server is contacted once, and not for every request, greatly improving performance. In order to use the JWT-based authentication method, set `authMethod` and `authJWTJWKS`:

```yml
authMethod: jwt
authJWTJWKS: http://my_identity_server/jwks_endpoint
authJWTClaimKey: mediamtx_permissions
```

Users are expected to pass the encoded JWT as token.

The JWT is expected to contain a claim, with a list of permissions in the same format as the one of user permissions:

```json
{
  "mediamtx_permissions": [
    {
      "action": "publish",
      "path": ""
    }
  ]
}
```

If the JWKS server uses TLS and has a self-signed or invalid TLS certificate, you can provide the fingerprint of the certificate to validate it anyway:

```yml
authMethod: jwt
authJWTJWKS: https://my_identity_server/jwks_endpoint
authJWTJWKSFingerprint: 33949e05fffb5ff3e8aa16f8213a6251b4d9363804ba53233c4da9a46d6f2739
authJWTClaimKey: mediamtx_permissions
```

The fingerprint can be obtained with:

```sh
openssl s_client -connect my_identity_server:443 </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
```

#### Keycloak setup

Here's a tutorial on how to setup the [Keycloak identity server](https://www.keycloak.org/) in order to provide JWTs.

1. Start Keycloak:

   ```sh
   docker run --name=keycloak -p 8080:8080 \
   -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin \
   quay.io/keycloak/keycloak:23.0.7 start-dev
   ```

2. Open the Keycloak web UI on http://localhost:8080, click on _Administration Console_ and log in.

3. Click on _master_ in the top left corner, _Create realm_, set realm name to `mediamtx`, _Create_.

4. Open page _Client scopes_, _Create client scope_, set name to `mediamtx`, _Save_.

5. Open tab _Mappers_, _Configure a new Mapper_, _User Attribute_:
   - Name: `mediamtx_permissions`
   - User Attribute: `mediamtx_permissions`
   - Token Claim Name: `mediamtx_permissions`
   - Claim JSON Type: `JSON`
   - Multivalued: `On`

   Save.

6. Open page _Clients_, _Create client_, set Client ID to `mediamtx`, _Next_, _Client authentication_ `On`, _Next_, _Save_.

7. Open tab _Credentials_, copy client secret somewhere.

8. Open tab _Client scopes_, set _Assigned type_ of all existing client scopes to _Optional_. This decreases the length of the JWT, since many clients impose limits on it.

9. In tab _Client scopes_, _Add client scope_, Select `mediamtx`, _Add_, _Default_.

10. Open page _Users_, _Add user_, Username `testuser`, _Create_, Tab _Credentials_, _Set password_, pick a password, _Save_.

11. Open tab _Attributes_, _Add an attribute_:
    - Key: `mediamtx_permissions`
    - Value: `{"action":"publish", "path": ""}`

    You can add as many attributes with key `mediamtx_permissions` as you want, each with a single permission in it.

12. In MediaMTX, use the following JWKS URL:

    ```yml
    authJWTJWKS: http://localhost:8080/realms/mediamtx/protocol/openid-connect/certs
    ```

13. Perform authentication on Keycloak:

    ```
    curl \
    -d "client_id=mediamtx" \
    -d "client_secret=$CLIENT_SECRET" \
    -d "username=$USER" \
    -d "password=$PASS" \
    -d "grant_type=password" \
    http://localhost:8080/realms/mediamtx/protocol/openid-connect/token
    ```

    The JWT is inside the `access_token` key of the response:

    ```json
    {
      "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICIyNzVjX3ptOVlOdHQ0TkhwWVk4Und6ZndUclVGSzRBRmQwY3lsM2wtY3pzIn0.eyJleHAiOjE3MDk1NTUwOTIsImlhdCI6MTcwOTU1NDc5MiwianRpIjoiMzE3ZTQ1NGUtNzczMi00OTM1LWExNzAtOTNhYzQ2ODhhYWIxIiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL3JlYWxtcy9tZWRpYW10eCIsImF1ZCI6ImFjY291bnQiLCJzdWIiOiI2NTBhZDA5Zi03MDgxLTQyNGItODI4Ni0xM2I3YTA3ZDI0MWEiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJtZWRpYW10eCIsInNlc3Npb25fc3RhdGUiOiJjYzJkNDhjYy1kMmU5LTQ0YjAtODkzZS0wYTdhNjJiZDI1YmQiLCJhY3IiOiIxIiwiYWxsb3dlZC1vcmlnaW5zIjpbIi8qIl0sInJlYWxtX2FjY2VzcyI6eyJyb2xlcyI6WyJvZmZsaW5lX2FjY2VzcyIsInVtYV9hdXRob3JpemF0aW9uIiwiZGVmYXVsdC1yb2xlcy1tZWRpYW10eCJdfSwicmVzb3VyY2VfYWNjZXNzIjp7ImFjY291bnQiOnsicm9sZXMiOlsibWFuYWdlLWFjY291bnQiLCJtYW5hZ2UtYWNjb3VudC1saW5rcyIsInZpZXctcHJvZmlsZSJdfX0sInNjb3BlIjoibWVkaWFtdHggcHJvZmlsZSBlbWFpbCIsInNpZCI6ImNjMmQ0OGNjLWQyZTktNDRiMC04OTNlLTBhN2E2MmJkMjViZCIsImVtYWlsX3ZlcmlmaWVkIjpmYWxzZSwibWVkaWFtdHhfcGVybWlzc2lvbnMiOlt7ImFjdGlvbiI6InB1Ymxpc2giLCJwYXRocyI6ImFsbCJ9XSwicHJlZmVycmVkX3VzZXJuYW1lIjoidGVzdHVzZXIifQ.Gevz7rf1qHqFg7cqtSfSP31v_NS0VH7MYfwAdra1t6Yt5rTr9vJzqUeGfjYLQWR3fr4XC58DrPOhNnILCpo7jWRdimCnbPmuuCJ0AYM-Aoi3PAsWZNxgmtopq24_JokbFArY9Y1wSGFvF8puU64lt1jyOOyxf2M4cBHCs_EarCKOwuQmEZxSf8Z-QV9nlfkoTUszDCQTiKyeIkLRHL2Iy7Fw7_T3UI7sxJjVIt0c6HCNJhBBazGsYzmcSQ_GrmhbUteMTg00o6FicqkMBe99uZFnx9wIBm_QbO9hbAkkzF923I-DTAQrFLxT08ESMepDwmzFrmnwWYBLE3u8zuUlCA",
      "expires_in": 300,
      "refresh_expires_in": 1800,
      "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICI3OTI3Zjg4Zi05YWM4LTRlNmEtYWE1OC1kZmY0MDQzZDRhNGUifQ.eyJleHAiOjE3MDk1NTY1OTIsImlhdCI6MTcwOTU1NDc5MiwianRpIjoiMGVhZWFhMWItYzNhMC00M2YxLWJkZjAtZjI2NTRiODlkOTE3IiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL3JlYWxtcy9tZWRpYW10eCIsImF1ZCI6Imh0dHA6Ly9sb2NhbGhvc3Q6ODA4MC9yZWFsbXMvbWVkaWFtdHgiLCJzdWIiOiI2NTBhZDA5Zi03MDgxLTQyNGItODI4Ni0xM2I3YTA3ZDI0MWEiLCJ0eXAiOiJSZWZyZXNoIiwiYXpwIjoibWVkaWFtdHgiLCJzZXNzaW9uX3N0YXRlIjoiY2MyZDQ4Y2MtZDJlOS00NGIwLTg5M2UtMGE3YTYyYmQyNWJkIiwic2NvcGUiOiJtZWRpYW10eCBwcm9maWxlIGVtYWlsIiwic2lkIjoiY2MyZDQ4Y2MtZDJlOS00NGIwLTg5M2UtMGE3YTYyYmQyNWJkIn0.yuXV8_JU0TQLuosNdp5xlYMjn7eO5Xq-PusdHzE7bsQ",
      "token_type": "Bearer",
      "not-before-policy": 0,
      "session_state": "cc2d48cc-d2e9-44b0-893e-0a7a62bd25bd",
      "scope": "mediamtx profile email"
    }
    ```

## Providing username and password

### RTSP

Prepend username and password and a `@` to the host:

```
rtsp rtsp://myuser:mypass@localhost:8554/mystream
```

### RTMP

Use the `user` and `pass` query parameters:

```
rtmp://localhost/mystream?user=myuser&pass=mypass
```

### SRT

Append username and password to `streamid`:

```
srt://localhost:8890?streamid=publish:mystream:user:pass&pkt_size=1316
```

### HLS and WebRTC

Username and password can be passed through the `Authorization: Basic` HTTP header:

```
Authorization: Basic base64(user:pass)
```

When using a web browser, a dialog is first shown to users, asking for credentials, and then the header is automatically inserted into every request. If you need to automatically fill credentials from a parent web page, see [Embed streams in a website](embed-streams-in-a-website).

If the `Authorization: Basic` header cannot be used (for instance, in software like OBS Studio, which only allows to provide a "Bearer Token"), credentials can be passed through the `Authorization: Bearer` header (i.e. the "Bearer Token" in OBS), where the value is the concatenation of username and password, separated by a colon:

```
Authorization: Bearer username:password
```

## Providing tokens / JWTs

### RTSP

Pass the token as password, with an arbitrary user:

```
rtsp://user:jwt@localhost:8554/mystream
```

WARNING: FFmpeg implementation of RTSP does not support passwords that are longer than 1024 characters, therefore you have to configure your identity server in order to produce JWTs that are shorter than this threshold.

### RTMP

Pass the token as password, with an arbitrary user:

```
rtmp://localhost/mystream?user=user&pass=jwt
```

WARNING: FFmpeg implementation of RTMP does not support passwords and query parameters that are longer than 1024 characters, therefore you have to configure your identity server in order to produce JWTs that are shorter than this threshold.

### SRT

Pass the token as password, with an arbitrary user:

```
srt://localhost:8890?streamid=publish:mystream:user:jwt&pkt_size=1316
```

WARNING: SRT does not support Stream IDs that are longer than 512 characters, therefore you have to configure your identity server in order to produce JWTs that are shorter than this threshold.

### HLS and WebRTC

The token can be passed through the `Authorization: Bearer` header:

```
Authorization: Bearer MY_JWT
```

In OBS Studio, this is the "Bearer Token" field.

If the `Authorization: Bearer` token cannot be directly provided (for instance, with web browsers that directly access _MediaMTX_ and show a credential dialog), you can pass the token as password, using an arbitrary user.

In web browsers, if you need to automatically fill credentials from a parent web page, see [Embed streams in a website](embed-streams-in-a-website).
