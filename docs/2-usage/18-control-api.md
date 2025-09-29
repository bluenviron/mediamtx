# Control API

The server can be queried and controlled with an API, that can be enabled by toggling the `api` parameter in the configuration:

```yml
api: yes
```

To obtain a list of of active paths, run:

```
curl http://127.0.0.1:9997/v3/paths/list
```

The control API is documented in the [Control API Reference page](/docs/references/control-api) and in the [OpenAPI / Swagger file](https://github.com/bluenviron/mediamtx/blob/{version_tag}/api/openapi.yaml).

Be aware that by default the Control API is accessible by localhost only; to increase visibility or add authentication, check [Authentication](authentication).
