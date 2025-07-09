// FILE: example/gnet/main.go
package main

import (
	"github.com/lixenwraith/log"
	"github.com/lixenwraith/log/compat"
	"github.com/panjf2000/gnet/v2"
)

// Example gnet event handler
type echoServer struct {
	gnet.BuiltinEventEngine
}

func (es *echoServer) OnTraffic(c gnet.Conn) gnet.Action {
	buf, _ := c.Next(-1)
	c.Write(buf)
	return gnet.None
}

func main() {
	// Method 1: Simple adapter
	logger := log.NewLogger()
	err := logger.InitWithDefaults(
		"directory=/var/log/gnet",
		"level=-4", // Debug level
		"format=json",
	)
	if err != nil {
		panic(err)
	}
	defer logger.Shutdown()

	gnetAdapter := compat.NewGnetAdapter(logger)

	// Configure gnet server with the logger
	err = gnet.Run(
		&echoServer{},
		"tcp://127.0.0.1:9000",
		gnet.WithMulticore(true),
		gnet.WithLogger(gnetAdapter),
		gnet.WithReusePort(true),
	)
	if err != nil {
		panic(err)
	}
}