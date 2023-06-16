package ps

import (
	"bytes"

	"github.com/gobwas/ws/wsutil"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/common"
	"m7s.live/engine/v4/util"
)

type PSSubscriber struct {
	Subscriber
}

func (ps *PSSubscriber) OnEvent(event any) {
	switch v := event.(type) {
	case *PSTrack:
		wsutil.WriteServerBinary(ps, util.ConcatBuffers(v.GetPSM()))
		enter := false
		go v.Play(ps.IO, func(data *common.DataFrame[*util.ListItem[util.Buffer]]) error {
			if !enter {
				if bytes.Compare(data.Data.Value[:3], []byte{0, 0, 1}) == 0 {
					enter = true
				} else {
					return nil
				}
			}
			// fmt.Printf("% 02X", data.Value.Value[:10])
			// fmt.Println()
			return wsutil.WriteServerBinary(ps, data.Data.Value)
		})
	default:
		ps.Subscriber.OnEvent(event)
	}
}
