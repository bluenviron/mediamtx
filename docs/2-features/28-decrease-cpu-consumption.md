# Decrease CPU consumption

When a stream is served to many readers or forwarded to many destinations, a significant share of CPU can be spent by the Go garbage collector. In [CPU reports](22-monitor-performance.md), it appears as `runtime.mallocgc` and `runtime.gcBgMarkWorker`.

The garbage collector can be tuned with these environment variables, provided by the Go runtime:

- `GOGC` sets how often memory is collected (default `100`). Higher values make collections less frequent, decreasing CPU usage and increasing memory usage.
- `GOMEMLIMIT` sets a soft memory limit. When memory usage approaches it, the garbage collector runs more frequently, regardless of `GOGC`.

For instance, when forwarding an 8 Mbps stream to 100 RTMP destinations, `GOGC=400` decreased CPU usage by about 30% and increased memory usage from about 70 MB to about 250 MB.

The variables can be set when launching the server:

```sh
docker run --rm -it \
-e GOGC=400 \
-e GOMEMLIMIT=1GiB \
--network=host \
bluenviron/mediamtx:1
```

More information is available in the [Go garbage collector guide](https://go.dev/doc/gc-guide).
