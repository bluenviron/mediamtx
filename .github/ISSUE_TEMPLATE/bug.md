---
name: Bug
about: Report a bug
title: ''
labels: ''
assignees: ''

---

<!--
To increase the chance of your issue getting fixed, open an issue FOR EACH problem. Please do not report multiple problems in a single issue, otherwise they'll probably never get ALL fixed.

Please include all sections of this template into your issue, or it will be automatically closed.
-->

## Which version are you using?

v0.0.0

## Which operating system are you using?

<!-- fill checkboxes with a x. Example: [x] Linux -->

- [ ] Linux amd64 standard
- [ ] Linux amd64 Docker
- [ ] Linux arm64 standard
- [ ] Linux arm64 Docker
- [ ] Linux arm7 standard
- [ ] Linux arm7 Docker
- [ ] Linux arm6 standard
- [ ] Linux arm6 Docker
- [ ] Windows amd64 standard
- [ ] Windows amd64 Docker (WSL backend)
- [ ] macOS amd64 standard
- [ ] macOS amd64 Docker
- [ ] Other (please describe)

## Describe the issue

Description

## Describe how to replicate the issue

<!--
the maintainers must be able to REPLICATE your issue to solve it - therefore, describe in a very detailed way how to replicate it.
-->

1. start the server
2. publish with ...
3. read with ...

## Did you attach the server logs?

<!--
Server logs are sometimes useful to identify the issue.
If you think this is the case, set the parameter 'logLevel' to 'debug' and attach the server logs.
-->

yes / no

## Did you attach a network dump?

<!--
If the bug arises when using rtsp-simple-server with an external hardware or software, the most helpful content you can provide is a dump of the data exchanged between the server and the target (network dump), that can be generated in this way:
1) Download wireshark (https://www.wireshark.org/)
2) Start capturing on the interface used for exchanging RTSP (if the server and the target software are both installed on your pc, the interface is probably "loopback", otherwise it's the one of your network card)
3) Start the server and replicate the issue
4) Stop capturing, save the result in .pcap format
5) Attach
-->

yes / no
