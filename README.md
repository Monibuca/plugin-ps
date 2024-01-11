# 简介
ps 插件主要用于接收MpegPS流, 公安部制定的GBT 28181标准广泛应用于安防领域，这个标准规定了传输的视音频数据要封装成PS流格式。PS格式（原名叫MPEG-PS）。
该插件用于GB28181插件的流接收解析，为GB28181插件提供TCP、UDP链接的服务。


## 插件地址

https://github.com/Monibuca/plugin-ps

# 使用方法

## 插件引入
```go
import (  
  _ "m7s.live/plugin/ps/v4" 
)
```
## 配置插件

### 默认配置

```yaml
ps:
  http: # 格式参考全局配置
  publish: # 格式参考全局配置
  subscribe: # 格式参考全局配置
  relaymode: 1 # 0:纯转发 1:转协议，不转发 2:转发并且转协议
```

### 配置 ps 插件
- 通用配置( 默认继承全局配置)

> 通用配置具体功能参考 [m7s 配置](https://monibuca.com/guide/config.html)
```yaml
ps:
  http: # 格式参考全局配置
  publish: # 格式参考全局配置
  subscribe: # 格式参考全局配置
```

- 基础配置
```yaml
ps:
  relaymode: 1 # 0:纯转发 1:转协议，不转发 2:转发并且转协议
```

relaymode 根据使用需要进行配置
```yaml
ps:
  relaymode: 0
```
以上配置后，为【纯转发】，当只需要获取ps流，不需要进行对PS流进行转协议时配置，m7s将不对流进行缓存、重排等一系列操作，因此延时更低。
应用场景举例：
  1、只需要GB级联；
  2、只需要拉取GB裸流；
需要注意：
  因为没有对流进行转协议，所以其他播放流的组件将不能播放；

```yaml
ps:
  relaymode: 1
```
以上配置后，为【转协议，不转发】，当不需要获取ps流，只需要播放流的时候配置，m7s将不对ps流进行保留。
需要注意：
  因为m7s没有对PS流进行保留，API中对PS流的读取接收操作将不能正常使用。


```yaml
ps:
  relaymode: 2
```
以上配置后，为【转发并且转协议】，m7s将保留PS流的同时对PS流进行协议解析，将占用更大的资源。
需要注意：
  当急需要对PS流进行播放，同时有需要使用GB级联时，必须将配置设为【转发并且转协议】。

# API

## 接收PS流
`/ps/api/receive?streamPath=xxx&ssrc=xxx&port=xxx&reuse=1&dump=xxx`
其中：
- reuse代表是否端口复用，如果使用端口复用，请务必确定设备发送的ssrc和ssrc参数一致，否则会出现混流的情况
- dump代表是否dump到文件，如果dump到文件，会在当前目录下生成一个以dump为名的文件夹，文件夹下面是以streamPath参数值为名的文件，文件内容从端口收到的数据[4byte 内容长度][2byte 相对时间][内容]
## 回放PS的dump文件

`/ps/api/replay?streamPath=xxx&dump=xxx`
- dump 代表需要回放的文件，默认是dump/ps
- streamPath 代表回放时生成的视频流的streamPath, 默认是replay/dump/ps (如果dump传了abc, 那么streamPath默认是replay/abc)

## 以ws协议读取PS流

`ws://[host]/ps/[streamPath]`

例如： ws://localhost:8080/ps/live/test

数据包含的是裸的PS数据，不包含rtp头