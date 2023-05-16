package ps

import (
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
		go v.Play(ps.IO, func(data *common.DataFrame[*util.ListItem[util.Buffer]]) error {
			return wsutil.WriteServerBinary(ps, data.Value.Value)
		})
	default:
		ps.Subscriber.OnEvent(event)
	}
}
