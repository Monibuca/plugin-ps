package mpegps

import (
	"io"
)

func (es *MpegPsEsStream) parsePESPacket(payload []byte) (result *MpegPsEsStream, err error) {
	if len(payload) < 4 {
		return nil, io.ErrShortBuffer
	}
	//data_alignment_indicator := (payload[0]&0b0001_0000)>>4 == 1
	flag := payload[1]
	ptsFlag := flag>>7 == 1
	dtsFlag := (flag&0b0100_0000)>>6 == 1
	pesHeaderDataLen := payload[2]
	if len(payload) < int(pesHeaderDataLen) {
		return nil, io.ErrShortBuffer
	}
	payload = payload[3:]
	extraData := payload[:pesHeaderDataLen]
	pts, dts := es.PTS, es.DTS
	if ptsFlag && len(extraData) > 4 {
		pts = uint32(extraData[0]&0b0000_1110) << 29
		pts |= uint32(extraData[1]) << 22
		pts |= uint32(extraData[2]&0b1111_1110) << 14
		pts |= uint32(extraData[3]) << 7
		pts |= uint32(extraData[4]) >> 1
		if dtsFlag && len(extraData) > 9 {
			dts = uint32(extraData[5]&0b0000_1110) << 29
			dts |= uint32(extraData[6]) << 22
			dts |= uint32(extraData[7]&0b1111_1110) << 14
			dts |= uint32(extraData[8]) << 7
			dts |= uint32(extraData[9]) >> 1
		} else {
			dts = pts
		}
	}
	if pts != es.PTS && es.Buffer.CanRead() {
		clone := *es
		result = &clone
		// fmt.Println("clone", es.PTS, es.Buffer[4]&0x0f)
		es.Buffer = nil
	}
	es.PTS, es.DTS = pts, dts
	// fmt.Println("append", es.PTS, payload[pesHeaderDataLen+4]&0x0f)
	es.Buffer = append(es.Buffer, payload[pesHeaderDataLen:]...)
	return
}
