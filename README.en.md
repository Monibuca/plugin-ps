_[简体中文](https://github.com/Monibuca/plugin-ps) | English_

# PS Plugin

enable receive MpegPS stream

## Plugin Address

https://github.com/Monibuca/plugin-ps

## Plugin Import
```go
    import (  _ "m7s.live/plugin/ps/v4" )
```

## Default Config

```yaml
ps:
  publish: # format refer to global config
  relaymode: 1 # 0:relay only 1:protocol transfar only 2:relay and protocol transfar
```

## API

### receive PS stream
`/ps/api/receive?streamPath=xxx&ssrc=xxx&port=xxx&reuse=1&dump=xxx`

- reuse means whether to reuse port, if reuse port, please make sure the ssrc from device is same as ssrc parameter, otherwise it will cause mixed stream
- dump means whether to dump to file, if dump to file, it will generate a folder named dump in current directory, the folder contains a file named by streamPath parameter, the file content is the data received from port [4byte content length][2byte relative time][content]
### replay PS dump file

`/ps/api/replay?streamPath=xxx&dump=xxx`
- dump means the file to replay, default is dump/ps
- streamPath means the streamPath of replayed video stream, default is replay/dump/ps (if dump is abc, then streamPath is replay/abc)