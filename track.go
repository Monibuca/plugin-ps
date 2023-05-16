package ps

import (
	"net"

	"m7s.live/engine/v4/common"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

type PSTrack struct {
	track.Data[*util.ListItem[util.Buffer]]
	PSM util.Buffer
}

func (ps *PSTrack) GetPSM() (result net.Buffers) {
	psmLen := ps.PSM.Len()
	return append(net.Buffers{[]byte{0, 0, 1, 0xbc, byte(psmLen >> 8), byte(psmLen)}}, ps.PSM)
}

func NewPSTrack(s common.IStream) *PSTrack {
	result := &PSTrack{}
	result.Init(20)
	result.SetStuff("ps", s)
	result.Reset = func(f *common.DataFrame[*util.ListItem[util.Buffer]]) {
		f.Value.Recycle()
	}
	s.AddTrack(result)
	return result
}
