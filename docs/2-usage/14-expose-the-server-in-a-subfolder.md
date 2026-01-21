# Expose the server in a subfolder

HTTP-based services (WebRTC, HLS, Control API, Playback Server, Metrics, pprof) can be exposed in a subfolder of an existing HTTP server or reverse proxy. The reverse proxy must be able to intercept HTTP requests addressed to _MediaMTX_ and corresponding responses, and perform the following changes:

- The subfolder path must be stripped from request paths. For instance, if the server is exposed behind `/subpath` and the reverse proxy receives a request with path `/subpath/mystream/index.m3u8`, this has to be changed into `/mystream/index.m3u8`.

- Any `Location` header in responses must be prefixed with the subfolder path. For instance, if the server is exposed behind `/subpath` and the server sends a response with `Location: /mystream/index.m3u8`, this has to be changed into `Location: /subfolder/mystream/index.m3u8`.

If _nginx_ is the reverse proxy, this can be achieved with the following configuration:

```
location /subpath/ {
    proxy_pass http://mediamtx-ip:8889/;
    proxy_redirect / /subpath/;
}
```

If _Apache HTTP Server_ is the reverse proxy, this can be achieved with the following configuration:

```
<Location /subpath>
    ProxyPass http://mediamtx-ip:8889
    ProxyPassReverse http://mediamtx-ip:8889
    Header edit Location ^(.*)$ "/subpath$1"
</Location>
```

If _Caddy_ is the reverse proxy, this can be achieved with the following configuration:

```
:80 {
    handle_path /subpath/* {
        reverse_proxy {
            to mediamtx-ip:8889
            header_down Location ^/ /subpath/
        }
    }
}
```
