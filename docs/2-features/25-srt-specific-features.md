# SRT-specific features

SRT is a protocol that can be used for publishing and reading streams. Regarding specific tasks, check out [Publish with SRT clients](../3-publish/02-srt-clients.md) and [Read with SRT clients](../4-read/02-srt.md). Features in this page are shared among both tasks.

## Standard stream ID syntax

In SRT, the stream ID is a string that is sent to the remote part in order to advertise what action the caller is going to do (publish or read), the path and the credentials. All this information has to be encoded into a single string. This server supports two stream ID syntaxes, a custom one (that is the one reported in the rest of the README) and also a [standard one](https://github.com/Haivision/srt/blob/master/docs/features/access-control.md) proposed by the authors of the protocol and enforced by some hardware. The standard syntax can be used in this way:

```
srt://localhost:8890?streamid=#!::m=publish,r=mypath,u=myuser,s=mypass&pkt_size=1316
```

Where:

- key `m` contains the action (`publish` or `request`)
- key `r` contains the path
- key `u` contains the username
- key `s` contains the password

## SRTLA (SRT Link Aggregation)

SRTLA is an extension of SRT that bonds multiple network connections (e.g. cellular + Wi-Fi) into a single reliable stream. This is commonly used by mobile streaming devices like BELABOX, IRL Pro, and Moblin.

When enabled, the server listens for SRTLA connections on a separate UDP port and transparently proxies them to the local SRT server.

To enable SRTLA, set the following in the configuration file:

```yaml
srtla: true
srtlaAddress: :8891
```

SRTLA clients connect to port `8891` and register multiple network links. The server aggregates incoming data packets and forwards them to the SRT server on port `8890`. SRT ACK packets are broadcast to all registered links for timely delivery, while other responses are sent only to the most recently active link.

Configuration reference:

- `srtla` — enable or disable the SRTLA listener (default: `true`)
- `srtlaAddress` — address of the SRTLA listener (default: `:8891`)
