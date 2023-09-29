package ps

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pion/rtp"
	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/config"
	"m7s.live/engine/v4/lang"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

type PSStream struct {
	Flag bool
	*PSPublisher
	net.Conn
}

type PSConfig struct {
	config.HTTP
	config.Publish
	config.Subscribe
	RelayMode int // 转发模式,0:转协议+不转发,1:不转协议+转发，2:转协议+转发
	streams   sync.Map
	shareTCP  sync.Map
	shareUDP  sync.Map
}

var conf = &PSConfig{}
var PSPlugin = InstallPlugin(conf)

func (c *PSConfig) OnEvent(event any) {
	switch event.(type) {
	case FirstConfig:
		lang.Merge("zh", map[string]string{
			"start receive ps stream from": "开始接收PS流来自",
			"stop receive ps stream from":  "停止接收PS流来自",
			"ssrc not found":               "未找到ssrc",
		})
	}
}

func (c *PSConfig) ServeTCP(conn net.Conn) {
	startTime := time.Now()
	reader := TCPRTP{
		Conn: conn,
	}
	tcpAddr := zap.String("tcp", conn.LocalAddr().String())
	var puber *PSPublisher
	var psStream *PSStream
	var cache net.Buffers
	err := reader.Start(func(data util.Buffer) (err error) {
		if psStream == nil {
			var rtpPacket rtp.Packet
			if err = rtpPacket.Unmarshal(data); err != nil {
				PSPlugin.Error("gb28181 decode rtp error:", zap.Error(err))
			}
			ssrc := rtpPacket.SSRC
			stream, loaded := conf.streams.LoadOrStore(ssrc, &PSStream{
				Conn: conn,
			})
			psStream = stream.(*PSStream)
			if loaded {
				if psStream.Conn != nil {
					return fmt.Errorf("ssrc conflict")
				}
			}
			return
		}
		if puber == nil {
			if psStream.PSPublisher != nil {
				puber = psStream.PSPublisher
				puber.Info("start receive ps stream from", tcpAddr)
				for _, buf := range cache {
					puber.PushPS(buf)
				}
				puber.PushPS(data)
				return
			} else {
				PSPlugin.Warn("publisher not found", zap.Uint32("ssrc", psStream.SSRC))
				cache = append(cache, append([]byte(nil), data...))
				if time.Since(startTime) > time.Second*5 {
					return fmt.Errorf("publisher not found")
				}
			}
		} else {
			puber.PushPS(data)
		}
		return
	})
	if puber != nil {
		puber.Stop(zap.Error(err))
		puber.Info("stop receive ps stream from", tcpAddr)
	}
}

func (c *PSConfig) ServeUDP(conn *net.UDPConn) {
	bufUDP := make([]byte, 1024*1024)
	udpAddr := zap.String("udp", conn.LocalAddr().String())
	var rtpPacket rtp.Packet
	PSPlugin.Info("start receive ps stream from", udpAddr)
	defer PSPlugin.Info("stop receive ps stream from", udpAddr)
	var lastSSRC uint32
	var lastPubber *PSPublisher
	for {
		// conn.SetReadDeadline(time.Now().Add(time.Second * 10))
		n, _, err := conn.ReadFromUDP(bufUDP)
		if err != nil {
			return
		}
		if err := rtpPacket.Unmarshal(bufUDP[:n]); err != nil {
			PSPlugin.Error("gb28181 decode rtp error:", zap.Error(err))
		}
		ssrc := rtpPacket.SSRC
		if lastSSRC != ssrc {
			if v, ok := conf.streams.Load(ssrc); ok {
				lastSSRC = ssrc
				lastPubber = v.(*PSStream).PSPublisher
			} else {
				PSPlugin.Error("ssrc not found", zap.Uint32("ssrc", ssrc))
				continue
			}
		}
		lastPubber.Packet = rtpPacket
		lastPubber.pushPS()
	}
}

func (c *PSConfig) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimPrefix(r.URL.Path, "/")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	var suber PSSubscriber
	suber.SetIO(conn)
	suber.SetParentCtx(r.Context())
	suber.ID = r.RemoteAddr

	if err = PSPlugin.Subscribe(streamPath, &suber); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var b []byte
	b, err = wsutil.ReadClientBinary(conn)
	var rtpPacket rtp.Packet
	if err == nil {
		dc := track.NewDataTrack[[]byte]("voice")
		dc.Attach(suber.Stream)
		for err == nil {
			err = rtpPacket.Unmarshal(b)
			if err == nil {
				dc.Push(rtpPacket.Payload)
			}
			b, err = wsutil.ReadClientBinary(conn)
		}
	}
	suber.Stop(zap.Error(err))
}
// Deprecated: 请使用PSPublisher的Receive
func Receive(streamPath, dump, port string, ssrc uint32, reuse bool) (err error) {
	if PSPlugin.Disabled {
		return fmt.Errorf("ps plugin is disabled")
	}
	var pubber PSPublisher
	stream, loaded := conf.streams.LoadOrStore(ssrc, &PSStream{Flag: true})
	psStream := stream.(*PSStream)
	if loaded {
		if psStream.Flag {
			return fmt.Errorf("ssrc %d already exists", ssrc)
		}
		psStream.Flag = true
	}
	if dump != "" {
		dump = filepath.Join(dump, streamPath)
		os.MkdirAll(filepath.Dir(dump), 0766)
		pubber.dump, err = os.OpenFile(dump, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
	}
	if err = PSPlugin.Publish(streamPath, &pubber); err == nil {
		psStream.PSPublisher = &pubber
		protocol, listenaddr, _ := strings.Cut(port, ":")
		if !strings.Contains(listenaddr, ":") {
			listenaddr = ":" + listenaddr
		}
		// TODO: 暂时通过streamPath来判断是否是录像流
		tmp := strings.Split(pubber.Stream.StreamName, "/")
		if len(tmp) > 1 {
			pubber.Stream.DelayCloseTimeout = time.Second * 10
			pubber.Stream.IdleTimeout = time.Second * 10
		}
		switch protocol {
		case "tcp":
			var tcpConf config.TCP
			tcpConf.ListenAddr = listenaddr
			if reuse {
				if _, ok := conf.shareTCP.LoadOrStore(listenaddr, &tcpConf); ok {
				} else {
					go func() {
						tcpConf.ListenTCP(PSPlugin, conf)
						conf.shareTCP.Delete(listenaddr)
					}()
				}
			} else {
				tcpConf.ListenNum = 1
				go tcpConf.ListenTCP(pubber, &pubber)
			}
		case "udp":
			if reuse {
				var udpConf struct {
					*net.UDPConn
				}
				if _, ok := conf.shareUDP.LoadOrStore(listenaddr, &udpConf); ok {
				} else {
					udpConn, err := util.ListenUDP(listenaddr, 1024*1024)
					if err != nil {
						PSPlugin.Error("udp listen error", zap.Error(err))
						return err
					}
					udpConf.UDPConn = udpConn
					go func() {
						conf.ServeUDP(udpConn)
						conf.shareUDP.Delete(listenaddr)
					}()
				}
			} else {
				udpConn, err := util.ListenUDP(listenaddr, 1024*1024)
				if err != nil {
					pubber.Stop()
					return err
				} else {
					go pubber.ServeUDP(udpConn)
				}
			}
		}
	}
	return
}

// 收流
func (c *PSConfig) API_receive(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	dump := query.Get("dump")
	streamPath := query.Get("streamPath")
	ssrc := query.Get("ssrc")
	port := query.Get("port")
	reuse := query.Get("reuse") // 是否复用端口
	if _ssrc, err := strconv.ParseInt(ssrc, 10, 0); err == nil {
		if err := Receive(streamPath, dump, port, uint32(_ssrc), reuse != ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			w.Write([]byte("ok"))
		}
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *PSConfig) API_replay(w http.ResponseWriter, r *http.Request) {
	dump := r.URL.Query().Get("dump")
	streamPath := r.URL.Query().Get("streamPath")
	if dump == "" {
		dump = "dump/ps"
	}
	f, err := os.OpenFile(dump, os.O_RDONLY, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		if streamPath == "" {
			if strings.HasPrefix(dump, "/") {
				streamPath = "replay" + dump
			} else {
				streamPath = "replay/" + dump
			}
		}
		var pub PSPublisher
		pub.SetIO(f)
		if err = PSPlugin.Publish(streamPath, &pub); err == nil {
			go pub.Replay(f)
			w.Write([]byte("ok"))
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
