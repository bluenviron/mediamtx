# Always-available streams

When the publisher or source of a stream is offline, the server can be configured to fill gaps in the stream with a video that is played on repeat until a publisher comes back online. Concatenation happens without decoding or re-encoding anything, using already-encoded packets in order to avoid any additional resource consumption.

This feature can be enabled by toggling the `alwaysAvailable` flag:

```yml
paths:
  mypath:
    alwaysAvailable: true
```
