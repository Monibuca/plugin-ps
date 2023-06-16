package ps

import (
	"net"

	"m7s.live/engine/v4/common"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

type PSTrack struct {
	track.RecycleData[*util.ListItem[util.Buffer]]
	PSM util.Buffer `json:"-" yaml:"-"`
}

func (ps *PSTrack) GetPSM() (result net.Buffers) {
	psmLen := ps.PSM.Len()
	return append(net.Buffers{[]byte{0, 0, 1, 0xbc, byte(psmLen >> 8), byte(psmLen)}}, ps.PSM)
}

func NewPSTrack(s common.IStream) *PSTrack {
	result := &PSTrack{}
	result.Init(1000)
	result.SetStuff("ps", s)
	s.AddTrack(result)
	return result
}
