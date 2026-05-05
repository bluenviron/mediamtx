# External path configuration

_MediaMTX_ normally requires every path to be declared in `mediamtx.yml` (either explicitly or via `all`/`all_others`). The **external path configuration** feature lets you delegate path resolution to an external HTTP service: whenever a client requests a path that matches only the catch-all `all` or `all_others` entry, the server queries your endpoint to obtain a custom configuration for that specific path.

This is useful when paths are created dynamically — for example in multi-tenant systems where each stream has its own configuration (source, recording policy, authentication requirements, etc.) stored in a database outside of MediaMTX.

## How it works

1. A client connects and requests a path (e.g. `rtsp://localhost:8554/live/camera42`).
2. MediaMTX runs its normal lookup:
   - If a **static** path config matches → used directly, external endpoint is **not** called.
   - If a **specific regexp** path config matches → used directly, external endpoint is **not** called.
   - If only `all` / `all_others` matches → external endpoint **is** called.
3. The endpoint receives a `GET` request at `{pathExternalConfURL}/{pathName}`.
4. Possible responses:
   - **HTTP 200** — body is a JSON object with path configuration fields (same fields as `pathDefaults`). Fields omitted in the response inherit from `pathDefaults`.
   - **HTTP 404** — path is unknown to the external service; MediaMTX falls back to `all`/`all_others`.
   - **Any other status** — treated as a transient error; a warning is logged and MediaMTX falls back to `all`/`all_others`.

## Configuration

```yml
# Enable dynamic path configuration fetching.
# Default: no
pathExternalConfEnabled: yes

# URL prefix of the external path configuration endpoint.
# MediaMTX appends /{pathName} to this URL.
# Required when pathExternalConfEnabled is yes.
pathExternalConfURL: http://my-backend/api/stream-configs
```

## Endpoint specification

### Request

```
GET {pathExternalConfURL}?path={pathName}&{originalQueryString}
```

| Query parameter | Description |
|-----------------|-------------|
| `path` | Name of the requested path (e.g. `live/camera42`) |
| `*` | All other query parameters from the client's original request are forwarded as-is. If the client sends a `path` parameter it is ignored to avoid collision. |

**Authentication headers** — MediaMTX forwards the connecting client's credentials to the endpoint according to the configured `authMethod`:

| `authMethod` | Header sent to endpoint |
|---|---|
| `internal` or `http` | `Authorization: Basic <base64(user:pass)>` |
| `jwt` | `Authorization: Bearer <token>` |

### Response

Return `Content-Type: application/json` with a JSON object. All fields are optional; omitted fields inherit from `pathDefaults`. Example:

```json
{
  "source": "rtsp://10.0.0.5:554/stream",
  "sourceOnDemand": true,
  "record": true,
  "recordPath": "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f",
  "runOnReady": "/usr/local/bin/notify.sh"
}
```

Return `404` if the path is not known. Any other non-200 status is treated as a transient error.

## Example

Suppose you have a database of cameras. Each camera is assigned a unique stream key. You want MediaMTX to accept any valid key and pull the corresponding RTSP source.

`mediamtx.yml`:

```yml
pathExternalConfEnabled: yes
pathExternalConfURL: http://localhost:8080/api/stream-configs

paths:
  all_others:
```

Backend (pseudo-code):

```
GET /api/stream-configs?path=camera42
→ 200 { "source": "rtsp://10.0.1.42:554/live", "sourceOnDemand": true }

GET /api/stream-configs?path=unknown-key
→ 404
```

When a client opens `rtsp://mediamtx:8554/camera42`:

1. `camera42` has no static config → matches `all_others`.
2. MediaMTX queries `http://localhost:8080/api/stream-configs/camera42`.
3. The backend returns the source URL.
4. MediaMTX uses that config to pull the RTSP feed and serve the client.

When a client opens `rtsp://mediamtx:8554/unknown-key`:

1. `unknown-key` matches `all_others`.
2. MediaMTX queries the backend → `404`.
3. MediaMTX falls back to the `all_others` config (default publisher mode).

## Notes

- The external endpoint is called **on every new connection** for paths that fall through to `all`/`all_others`. Responses are **not cached**; add caching in your backend if needed.
- The returned configuration is **not validated** by MediaMTX the same way static configs are. Make sure the JSON fields are correct — invalid values may cause unexpected behavior.
- Hot-reloading MediaMTX configuration does **not** re-read external path configs for already-established paths.
- The `pathExternalConfURL` setting does **not** support hot-reload; restart MediaMTX to change it.
