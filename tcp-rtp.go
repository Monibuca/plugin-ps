package ps

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"

	"m7s.live/engine/v4/util"
)

type TCPRTP struct {
	net.Conn
}

func (t *TCPRTP) Start(onRTP func(util.Buffer) error) (err error) {
	reader := bufio.NewReader(t.Conn)
	rtpLenBuf := make([]byte, 4)
	buffer := make(util.Buffer, 1024)
	for err == nil {
		if _, err = io.ReadFull(reader, rtpLenBuf); err != nil {
			return
		}
		rtpLen := int(binary.BigEndian.Uint16(rtpLenBuf[:2]))
		if rtpLenBuf[2]>>6 != 2 || rtpLenBuf[2]&0x0f > 15 || rtpLenBuf[3]&0x7f > 127 {
			buffer.Write(rtpLenBuf)
			for i := 12; i < buffer.Len()-2; i++ {
				if buffer[i]>>6 != 2 || buffer[i]&0x0f > 15 || buffer[i+1]&0x7f > 127 {
					continue
				}
				rtpLen = int(binary.BigEndian.Uint16(buffer[i-2 : i]))
				if buffer.Len() < rtpLen {
					copy(buffer, buffer[i:])
					if _, err = io.ReadFull(reader, buffer[buffer.Len():]); err != nil {
						return
					}
					err = onRTP(buffer)
					break
				} else {
					err = onRTP(buffer.SubBuf(i, rtpLen))
					if err != nil {
						return
					}
					i += rtpLen
					if buffer.Len() > i+1 {
						i += 2
					} else if buffer.Len() > i {
						reader.UnreadByte()
						break
					} else {
						break
					}
					i--
				}
			}
		} else {
			buffer.Relloc(rtpLen)
			copy(buffer, rtpLenBuf[2:])
			if _, err = io.ReadFull(reader, buffer[2:]); err != nil {
				return
			}
			err = onRTP(buffer)
		}
	}
	return
}
