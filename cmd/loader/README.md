# MediaMTX config path loader

This is a small CLI that reads the top-level `paths:` section from a MediaMTX YAML config file (for example `mediamtx2.yml`) and loads those paths into a **running** MediaMTX instance via the HTTP API.

It uses these endpoints:

- `GET /v3/config/paths/get/<name>` (detect whether the path already exists)
- `POST /v3/config/paths/add/<name>` (add)
- `PATCH /v3/config/paths/patch/<name>` (patch)
- `POST /v3/config/paths/replace/<name>` (replace)

## Prerequisites

- MediaMTX must have the API enabled (in the MediaMTX config):

```yaml
api: yes
```

## Run

From the repo root:

```bash
# Example file:
#   cmd/loader/example_testpattern.yml
#
# Add only: create missing paths, skip existing ones
go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode add

# Patch existing paths
go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode patch

# Replace existing paths
go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode replace
```

If your API requires basic auth:

```bash
go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode add -user USER -pass PASS
```

Dry-run (print actions, don’t call the API):

```bash
go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode replace -dry-run
```

To load the included test-pattern example:

```bash
go run ./cmd/loader -config cmd/loader/example_testpattern.yml -api http://localhost:9997 -mode replace
```

The included example (`cmd/loader/example_testpattern.yml`) defines a single path `test/pattern` that:

- publishes an FFmpeg `testsrc2` stream to `rtsp://127.0.0.1:8554/test/pattern`
- encodes H.264 at ~`3Mbps` (`-b:v 3M`)
- sets `metadata.expectedBitrate: 3Mbps`

## Output

The loader prints **one block per non-skip action** (`ADD`, `PATCH`, `REPLACE`) with request + response details:

- `request.method`
- `request.url`
- `request.body` (pretty JSON)
- `response.status`
- `response.body`

When `-mode add` and the path already exists, it is **skipped silently**.

## Notes

- The YAML is parsed with `yaml.UnmarshalStrict` (duplicates / invalid YAML will fail fast).
- Path names are URL-escaped while preserving `/` (e.g. `test/pattern` stays a single name).
- On API validation errors, the server response is printed in `response.body`.

