# Monitor performance

CPU and memory consumption can be monitored over time through an integrated performance monitor, that produces reports in a format compatible with [pprof](https://github.com/google/pprof), which is a tool included in any Go installation.

The performance monitor can be enabled by setting `pprof: yes` in the configuration.

Reports can be extracted and displayed by using the `go tool pprof` command.

Occupied memory can be analyzed by running:

```sh
go tool pprof -text http://localhost:9999/debug/pprof/heap
```

Obtaining:

```
Fetching profile over HTTP from http://localhost:9999/debug/pprof/heap
Saved profile in /home/xxx/pprof/pprof.mediamtx.alloc_objects.alloc_space.inuse_objects.inuse_space.007.pb.gz
File: mediamtx
Build ID: dfb7c97dbb5e9ce59438172269231b35a873e3e9
Type: inuse_space
Time: Sep 99, 9999 at 12:00pm (CEST)
Showing nodes accounting for 5145.10kB, 100% of 5145.10kB total
      flat  flat%   sum%        cum   cum%
    2052kB 39.88% 39.88%     2052kB 39.88%  runtime.allocm
  525.43kB 10.21% 50.09%   525.43kB 10.21%  github.com/go-playground/validator/v10.map.init.7
  518.65kB 10.08% 60.18%   518.65kB 10.08%  github.com/go-playground/validator/v10.map.init.3
  512.75kB  9.97% 70.14%   512.75kB  9.97%  github.com/bluenviron/gortsplib/v4.(*serverUDPListener).run.func1 (inline)
  512.22kB  9.96% 80.10%   512.22kB  9.96%  runtime.malg
  512.02kB  9.95% 90.05%   512.02kB  9.95%  crypto/tls.init
  512.02kB  9.95%   100%   512.02kB  9.95%  internal/abi.NewName
         0     0%   100%   512.75kB  9.97%  github.com/bluenviron/gortsplib/v4.(*serverUDPListener).run
         0     0%   100%   512.02kB  9.95%  github.com/bluenviron/mediamtx/internal/conf.init
         0     0%   100%   512.02kB  9.95%  github.com/bluenviron/mediamtx/internal/conf.init.func2 (inline)
         0     0%   100%  1044.08kB 20.29%  github.com/go-playground/validator/v10.init
         0     0%   100%   512.02kB  9.95%  reflect.StructOf
         0     0%   100%   512.02kB  9.95%  reflect.newName (inline)
         0     0%   100%   512.02kB  9.95%  reflect.runtimeStructField
         0     0%   100%  2068.13kB 40.20%  runtime.doInit (inline)
         0     0%   100%  2068.13kB 40.20%  runtime.doInit1
         0     0%   100%  2068.13kB 40.20%  runtime.main
         0     0%   100%      513kB  9.97%  runtime.mcall
         0     0%   100%     1539kB 29.91%  runtime.mstart
         0     0%   100%     1539kB 29.91%  runtime.mstart0
         0     0%   100%     1539kB 29.91%  runtime.mstart1
         0     0%   100%     2052kB 39.88%  runtime.newm
         0     0%   100%   512.22kB  9.96%  runtime.newproc.func1
         0     0%   100%   512.22kB  9.96%  runtime.newproc1
         0     0%   100%      513kB  9.97%  runtime.park_m
         0     0%   100%     2052kB 39.88%  runtime.resetspinning
         0     0%   100%     2052kB 39.88%  runtime.schedule
         0     0%   100%     2052kB 39.88%  runtime.startm
         0     0%   100%   512.22kB  9.96%  runtime.systemstack
         0     0%   100%     2052kB 39.88%  runtime.wakep
```

Consumed CPU can be analyzed by running:

```sh
go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=15
```

Obtaining:

```
Fetching profile over HTTP from http://localhost:9999/debug/pprof/profile?seconds=15
Saved profile in /home/xxx/pprof/pprof.mediamtx.samples.cpu.003.pb.gz
File: mediamtx
Build ID: dfb7c97dbb5e9ce59438172269231b35a873e3e9
Type: cpu
Time: Sep 99, 9999 at 12:00pm (CEST)
Duration: 15s, Total samples = 0
Showing nodes accounting for 70ms, 100% of 70ms total
      flat  flat%   sum%        cum   cum%
      30ms 42.86% 42.86%       30ms 42.86%  internal/runtime/syscall.Syscall6
      10ms 14.29% 57.14%       10ms 14.29%  runtime.(*consistentHeapStats).acquire
      10ms 14.29% 71.43%       10ms 14.29%  runtime.futex
      10ms 14.29% 85.71%       10ms 14.29%  runtime.mapIterStart
      10ms 14.29%   100%       10ms 14.29%  runtime.netpollblockcommit
         0     0%   100%       10ms 14.29%  github.com/bluenviron/gortsplib/v4.(*serverSessionFormat).readPacketRTP
         0     0%   100%       10ms 14.29%  github.com/bluenviron/gortsplib/v4.(*serverSessionMedia).readPacketRTPUDPRecord
         0     0%   100%       50ms 71.43%  github.com/bluenviron/gortsplib/v4.(*serverUDPListener).run
         0     0%   100%       10ms 14.29%  github.com/bluenviron/gortsplib/v4.(*serverUDPListener).run.func2
         0     0%   100%       10ms 14.29%  github.com/bluenviron/mediamtx/internal/protocols/rtsp.ToStream.func2
         0     0%   100%       10ms 14.29%  github.com/bluenviron/mediamtx/internal/stream.(*Stream).WriteRTPPacket
         0     0%   100%       10ms 14.29%  github.com/bluenviron/mediamtx/internal/stream.(*streamFormat).writeRTPPacket
         0     0%   100%       10ms 14.29%  github.com/bluenviron/mediamtx/internal/stream.(*streamFormat).writeUnitInner
         0     0%   100%       30ms 42.86%  internal/poll.(*FD).ReadFromInet6
         0     0%   100%       10ms 14.29%  internal/runtime/syscall.EpollWait
         0     0%   100%       40ms 57.14%  net.(*UDPConn).ReadFrom
         0     0%   100%       30ms 42.86%  net.(*UDPConn).readFrom
         0     0%   100%       30ms 42.86%  net.(*UDPConn).readFromUDP
         0     0%   100%       30ms 42.86%  net.(*netFD).readFromInet6
         0     0%   100%       10ms 14.29%  runtime.(*mcache).nextFree
         0     0%   100%       10ms 14.29%  runtime.(*mcache).refill
         0     0%   100%       10ms 14.29%  runtime.entersyscall
         0     0%   100%       10ms 14.29%  runtime.entersyscall_sysmon
         0     0%   100%       10ms 14.29%  runtime.findRunnable
         0     0%   100%       10ms 14.29%  runtime.futexwakeup
         0     0%   100%       10ms 14.29%  runtime.mallocgc
         0     0%   100%       10ms 14.29%  runtime.mallocgcSmallScanNoHeader
         0     0%   100%       20ms 28.57%  runtime.mcall
         0     0%   100%       10ms 14.29%  runtime.netpoll
         0     0%   100%       10ms 14.29%  runtime.newobject
         0     0%   100%       10ms 14.29%  runtime.notewakeup
         0     0%   100%       20ms 28.57%  runtime.park_m
         0     0%   100%       10ms 14.29%  runtime.reentersyscall
         0     0%   100%       10ms 14.29%  runtime.schedule
         0     0%   100%       10ms 14.29%  runtime.systemstack
         0     0%   100%       20ms 28.57%  syscall.RawSyscall6
         0     0%   100%       30ms 42.86%  syscall.Syscall6
         0     0%   100%       30ms 42.86%  syscall.recvfrom
         0     0%   100%       30ms 42.86%  syscall.recvfromInet6
```

Active routines can be listed by running:

```sh
go tool pprof -text http://localhost:9999/debug/pprof/goroutine
```

Obtaining:

```
Fetching profile over HTTP from http://localhost:9999/debug/pprof/goroutine
Saved profile in /home/xxx/pprof/pprof.mediamtx.goroutine.044.pb.gz
File: mediamtx
Build ID: dfb7c97dbb5e9ce59438172269231b35a873e3e9
Type: goroutine
Time: Sep 99, 9999 at 12:00pm (CEST)
Showing nodes accounting for 27, 100% of 27 total
      flat  flat%   sum%        cum   cum%
        25 92.59% 92.59%         25 92.59%  runtime.gopark
         1  3.70% 96.30%          1  3.70%  runtime.goroutineProfileWithLabels
         1  3.70%   100%          1  3.70%  runtime.notetsleepg
         0     0%   100%          1  3.70%  github.com/bluenviron/gortsplib/v4.(*Server).Wait (inline)
         0     0%   100%          1  3.70%  github.com/bluenviron/gortsplib/v4.(*Server).run
         0     0%   100%          1  3.70%  github.com/bluenviron/gortsplib/v4.(*Server).runInner
         0     0%   100%          1  3.70%  github.com/bluenviron/gortsplib/v4.(*serverTCPListener).run
         0     0%   100%          2  7.41%  github.com/bluenviron/gortsplib/v4.(*serverUDPListener).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/confwatcher.(*ConfWatcher).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/core.(*Core).Wait (inline)
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/core.(*Core).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/core.(*pathManager).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/protocols/httpp.(*handlerExitOnPanic).ServeHTTP
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/protocols/httpp.(*handlerFilterRequests).ServeHTTP
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/protocols/httpp.(*handlerLogger).ServeHTTP
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/protocols/httpp.(*handlerServerHeader).ServeHTTP
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/recordcleaner.(*Cleaner).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/hls.(*Server).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/rtmp.(*Server).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/rtmp.(*listener).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/rtmp.(*listener).runInner
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/rtsp.(*Server).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/rtsp.(*Server).run.func1
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/srt.(*Server).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/srt.(*listener).run
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/srt.(*listener).runInner
         0     0%   100%          1  3.70%  github.com/bluenviron/mediamtx/internal/servers/webrtc.(*Server).run
         0     0%   100%          1  3.70%  github.com/datarhei/gosrt.(*listener).Accept2
         0     0%   100%          1  3.70%  github.com/datarhei/gosrt.(*listener).reader
         0     0%   100%          1  3.70%  github.com/datarhei/gosrt.Listen.func1
         0     0%   100%          1  3.70%  github.com/fsnotify/fsnotify.(*inotify).readEvents
         0     0%   100%          1  3.70%  github.com/gin-contrib/pprof.RouteRegister.WrapH.func9
         0     0%   100%          1  3.70%  github.com/gin-gonic/gin.(*Context).Next (inline)
         0     0%   100%          1  3.70%  github.com/gin-gonic/gin.(*Engine).ServeHTTP
         0     0%   100%          1  3.70%  github.com/gin-gonic/gin.(*Engine).handleHTTPRequest
         0     0%   100%          1  3.70%  github.com/pion/ice/v4.(*UDPMuxDefault).connWorker
         0     0%   100%          5 18.52%  internal/poll.(*FD).Accept
         0     0%   100%          2  7.41%  internal/poll.(*FD).Read
         0     0%   100%          4 14.81%  internal/poll.(*FD).ReadFromInet6
         0     0%   100%         11 40.74%  internal/poll.(*pollDesc).wait
         0     0%   100%         11 40.74%  internal/poll.(*pollDesc).waitRead (inline)
         0     0%   100%         11 40.74%  internal/poll.runtime_pollWait
         0     0%   100%          1  3.70%  main.main
         0     0%   100%          5 18.52%  net.(*TCPListener).Accept
         0     0%   100%          5 18.52%  net.(*TCPListener).accept
         0     0%   100%          4 14.81%  net.(*UDPConn).ReadFrom
         0     0%   100%          4 14.81%  net.(*UDPConn).readFrom
         0     0%   100%          4 14.81%  net.(*UDPConn).readFromUDP
         0     0%   100%          1  3.70%  net.(*conn).Read
         0     0%   100%          1  3.70%  net.(*netFD).Read
         0     0%   100%          5 18.52%  net.(*netFD).accept
         0     0%   100%          4 14.81%  net.(*netFD).readFromInet6
         0     0%   100%          3 11.11%  net/http.(*Server).Serve
         0     0%   100%          1  3.70%  net/http.(*conn).serve
         0     0%   100%          1  3.70%  net/http.(*connReader).backgroundRead
         0     0%   100%          1  3.70%  net/http.serverHandler.ServeHTTP
         0     0%   100%          1  3.70%  net/http/pprof.handler.ServeHTTP
         0     0%   100%          1  3.70%  os.(*File).Read
         0     0%   100%          1  3.70%  os.(*File).read (inline)
         0     0%   100%          1  3.70%  os/signal.loop
         0     0%   100%          1  3.70%  os/signal.signal_recv
         0     0%   100%          1  3.70%  runtime.chanrecv
         0     0%   100%          1  3.70%  runtime.chanrecv1
         0     0%   100%          1  3.70%  runtime.goparkunlock (inline)
         0     0%   100%          1  3.70%  runtime.main
         0     0%   100%         11 40.74%  runtime.netpollblock
         0     0%   100%          1  3.70%  runtime.pprof_goroutineProfileWithLabels
         0     0%   100%         12 44.44%  runtime.selectgo
         0     0%   100%          1  3.70%  runtime.semacquire1
         0     0%   100%          1  3.70%  runtime/pprof.(*Profile).WriteTo
         0     0%   100%          1  3.70%  runtime/pprof.writeGoroutine
         0     0%   100%          1  3.70%  runtime/pprof.writeRuntimeProfile
         0     0%   100%          1  3.70%  sync.(*WaitGroup).Wait
         0     0%   100%          1  3.70%  sync.runtime_SemacquireWaitGroup
```
