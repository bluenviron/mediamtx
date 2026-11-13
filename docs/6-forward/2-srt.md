# SRT

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: srt://host:port?streamid=streamid
```
