# SRT-specific features

SRT is a protocol that can be used for publishing and reading streams. Regarding specific tasks, see [Publish](publish#srt-clients) and [Read](read#srt). Features in these page are shared among both tasks.

## Standard stream ID syntax

In SRT, the stream ID is a string that is sent to the remote part in order to advertise what action the caller is gonna do (publish or read), the path and the credentials. All these informations have to be encoded into a single string. This server supports two stream ID syntaxes, a custom one (that is the one reported in rest of the README) and also a [standard one](https://github.com/Haivision/srt/blob/master/docs/features/access-control.md) proposed by the authors of the protocol and enforced by some hardware. The standard syntax can be used in this way:

```
srt://localhost:8890?streamid=#!::m=publish,r=mypath,u=myuser,s=mypass&pkt_size=1316
```

Where:

- key `m` contains the action (`publish` or `request`)
- key `r` contains the path
- key `u` contains the username
- key `s` contains the password
