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

If _MediaMTX_ is also running as a [system service](start-on-boot), log entries can be queried by using:

```sh
journalctl -u mediamtx
```

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
