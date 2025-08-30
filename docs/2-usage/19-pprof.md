# Pprof

A performance monitor, compatible with pprof, can be enabled with the parameter `pprof: yes`; then the server can be queried for metrics with pprof-compatible tools, like:

```
go tool pprof -text http://localhost:9999/debug/pprof/goroutine
go tool pprof -text http://localhost:9999/debug/pprof/heap
go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=30
```
