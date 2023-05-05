package pluginps

import (
	. "m7s.live/engine/v4"
)

type PSConfig struct {
	Publisher
}
var conf = &PSConfig{}
var PSPlugin = InstallPlugin(conf)

func (c *PSConfig) OnEvent(event any) {
	
}