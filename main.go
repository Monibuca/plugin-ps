package ps

import (
	"encoding/binary"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pion/rtp"
	"go.uber.org/zap"
	"io"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/config"
	"m7s.live/engine/v4/lang"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

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
	var err error
	ps := make(util.Buffer, 1024)
	tcpAddr := zap.String("tcp", conn.LocalAddr().String())

	rtpLen := make([]byte, 2)
	var puber *PSPublisher
	if _, err = io.ReadFull(conn, rtpLen); err != nil {
		return
	}
	ps.Relloc(int(binary.BigEndian.Uint16(rtpLen)))
	if _, err = io.ReadFull(conn, ps); err != nil {
		return
	}
	var rtpPacket rtp.Packet
	if err := rtpPacket.Unmarshal(ps); err != nil {
		PSPlugin.Error("gb28181 decode rtp error:", zap.Error(err))
	}
	ssrc := rtpPacket.SSRC
	if v, ok := conf.streams.Load(ssrc); ok {
		puber = v.(*PSPublisher)
		puber.Info("start receive ps stream from", tcpAddr)
		defer puber.Info("stop receive ps stream from", tcpAddr)
		defer puber.Stop()
		for err == nil {
			puber.PushPS(ps)
			if _, err = io.ReadFull(conn, rtpLen); err != nil {
				return
			}
			ps.Relloc(int(binary.BigEndian.Uint16(rtpLen)))
			if _, err = io.ReadFull(conn, ps); err != nil {
				return
			}
		}
	} else {
		PSPlugin.Error("ssrc not found", zap.Uint32("ssrc", ssrc))
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
		//conn.SetReadDeadline(time.Now().Add(time.Second * 10))
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
				lastPubber = v.(*PSPublisher)
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

	defer suber.Stop()
	b, err := wsutil.ReadClientBinary(conn)
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
	} else {
		// baseStream.Error("receive", zap.Error(err))
	}
}

func Receive(streamPath, dump, port string, ssrc uint32, reuse bool) (err error) {
	var pubber PSPublisher
	if _, loaded := conf.streams.LoadOrStore(ssrc, &pubber); loaded {
		return fmt.Errorf("ssrc %d already exists", ssrc)
	} else {
		if dump != "" {
			dump = filepath.Join(dump, streamPath)
			os.MkdirAll(filepath.Dir(dump), 0766)
			pubber.dump, err = os.OpenFile(dump, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return
			}
		}
		if err = PSPlugin.Publish(streamPath, &pubber); err == nil {
			protocol, listenaddr, _ := strings.Cut(port, ":")
			if !strings.Contains(listenaddr, ":") {
				listenaddr = ":" + listenaddr
			}
			switch protocol {
			case "tcp":
				var tcpConf config.TCP
				tcpConf.ListenAddr = listenaddr
				if reuse {
					if _, ok := conf.shareTCP.LoadOrStore(listenaddr, &tcpConf); ok {
					} else {
						conf.streams.Store(ssrc, &pubber)
						go tcpConf.ListenTCP(PSPlugin, conf)
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
						conf.streams.Store(ssrc, &pubber)
						go conf.ServeUDP(udpConn)
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
