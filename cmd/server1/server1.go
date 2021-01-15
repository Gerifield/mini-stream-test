package main

import (
	"fmt"
	"log"
	"net"

	"github.com/torresjeff/rtmp"
	"github.com/torresjeff/rtmp/config"
	"github.com/torresjeff/rtmp/rand"
	"github.com/torresjeff/rtmp/video"
)

func main() {

	listener, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalln(err)
	}

	// broadcaster stores information about all running subscribers in a global object.
	context := rtmp.NewInMemoryContext()
	broadcaster := rtmp.NewBroadcaster(context)

	// Loop infinitely, accepting any incoming connection. Every new connection will create a new session.
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("accepted incoming connection from", conn.RemoteAddr().String())

		// Create a new session from the new connection (basically a wrapper of the connection + other data)
		sess := rtmp.NewSession(rand.GenerateSessionId(), &conn, broadcaster)

		// Listen on video
		sess.OnVideo = func(frameType video.FrameType, codec video.Codec, payload []byte, timestamp uint32) {
			fmt.Println("frameType", frameType, "codec", codec)
			fmt.Println("TS", timestamp)
			fmt.Println("Payload size", len(payload))
		}

		go func() {
			err := sess.Run()
			if config.Debug {
				fmt.Println("rtmp: server: session closed, err:", err)
			}
		}()
	}
}
