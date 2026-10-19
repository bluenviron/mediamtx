# Logging

## Log verbosity

Log verbosity can be set with the `logLevel` parameter:

```yml
# Verbosity of the program; available values are "error", "warn", "info", "debug".
logLevel: info
```

## Log destinations

Log entries can be sent to multiple destinations. By default, they are printed on the console (stdout).

It is possible to write logs to a file by using these parameters:

```yml
# Destinations of log messages; available values are "stdout", "file" and "syslog".
logDestinations: [file]
# If "file" is in logDestinations, this is the file which will receive the logs.
logFile: mediamtx.log
```

It is possible to write logs to the system logging server (syslog) by using these parameters:

```yml
# Destinations of log messages; available values are "stdout", "file" and "syslog".
logDestinations: [syslog]
# If "syslog" is in logDestinations, use prefix for logs.
sysLogPrefix: mediamtx
```

Log entries can be queried by using:

```sh
journalctl SYSLOG_IDENTIFIER=mediamtx
```

If _MediaMTX_ is also running as a [system service](16-start-on-boot.md), log entries can be queried by using:

```sh
journalctl -u mediamtx
```

## Structured logging

Log collectors (like Loki, Logstash, CloudWatch and fluentd) parse logs in a more reliable way if they are fed with entries in structured format (JSONL). This can be enabled with the `logStructured` parameter:

```yml
# When destination is "stdout" or "file", emit logs in structured format (JSONL).
logStructured: true
```

Obtaining:

```
{"timestamp":"20XX-YY-ZZT10:45:05.999999999+01:00","level":"INF","message":"[RTSP] listener opened on :8554 (TCP/RTSP), :8000 (UDP/RTP), :8001 (UDP/RTCP)"}
{"timestamp":"20XX-YY-ZZT10:45:05.999999999+01:00","level":"INF","message":"[RTMP] listener opened on :1935"}
{"timestamp":"20XX-YY-ZZT10:45:05.999999999+01:00","level":"INF","message":"[HLS] listener opened on :8888"}
{"timestamp":"20XX-YY-ZZT10:45:05.999999999+01:00","level":"INF","message":"[WebRTC] listener opened on :8889 (TCP/HTTP), :8189 (UDP/ICE)"}
{"timestamp":"20XX-YY-ZZT10:45:05.999999999+01:00","level":"INF","message":"[SRT] listener opened on :8890 (UDP)"}
```

## Hook output

The output of [hooks](19-hooks.md) is inherited from the standard streams of the server, therefore it doesn't follow `logLevel`, `logDestinations` and `logStructured`. It can be routed to the server log with the `logHookOutput` parameter:

```yml
# Route the output of hooks (runOnInit, runOnDemand, ...) to the server log,
# instead of letting it inherit the standard streams of the server.
logHookOutput: true
```

Standard output and standard error are merged and logged at info level, one entry per line; a line longer than 4096 bytes is split into several entries, and empty lines are skipped. A carriage return without a newline does not end an entry, therefore progress output that a program rewrites in place, such as the statistics of FFmpeg, is collected into long entries; FFmpeg stops printing it with `-nostats`. Both streams share a single pipe, so output written to them at the same time can interleave. When `logStructured` is enabled, a hook that writes non-printable characters or invalid UTF-8 can produce log entries that are not valid JSON. A change of this parameter at runtime applies to hooks started, or restarted, after the change; hooks already running keep their current routing.

When this is enabled, after a hook exits the server waits up to 1 second for the output that is still in transit, and discards whatever is left after that. Output can therefore be truncated in two cases: when the hook leaves background processes behind (`sh -c '... &'`), that keep the output pipes open, and when the log destinations are slower than the output the hook produced. In the latter case, the tail of the output of a hook that behaved correctly can be lost too. When the limit expires the output pipes are closed: a background process that keeps writing to them fails on its next write (with `SIGPIPE` or `EPIPE` on POSIX systems, where the default action terminates it), while with `logHookOutput` disabled it would keep writing to the inherited standard streams of the server. The limit doesn't apply to a log write that is already in progress: such a write is never interrupted and can take longer than the limit itself. Hook output shares the log destinations with the server, therefore a hook that writes faster than the destinations can accept also delays the log entries of the server.

## Log file rotation

The log file can be periodically rotated or truncated by using an external utility.

On most Linux distributions, the `logrotate` utility is in charge of managing log files. It can be configured to handle the _MediaMTX_ log file too by creating a configuration file, placed in `/etc/logrotate.d/mediamtx`, with this content:

```
/my/mediamtx/path/mediamtx.log {
    daily
    copytruncate
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
}
```

This file will rotate the log file every day, adding a `.NUMBER` suffix to older copies:

```
mediamtx.log.1
mediamtx.log.2
mediamtx.log.3
...
```
