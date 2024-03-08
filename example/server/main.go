package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	gotcpws "github.com/sazonovItas/go-tcpws"
)

var portFlag = flag.String("p", "8080", "port to listen")

func main() {
	flag.Parse()

	listener, err := net.Listen("tcp", ":"+*portFlag)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	log.Println("server listen on address:", listener.Addr())

	log.Fatal(ListenAndServe(listener))
}

func ListenAndServe(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			if err == net.ErrClosed {
				return err
			}
			continue
		}

		log.Println("New connection on address:", c.RemoteAddr())
		conn := gotcpws.NewFrameConnection(c, nil, nil, 0)
		go Serve(conn)
	}
}

func Serve(conn *gotcpws.Conn) {
	defer conn.Close()

	var err error
	var msg []byte
	for {
		msg, err = conn.ReadFrame()
		if err != nil {
			break
		}

		fmt.Println(string(msg))

		_, err = conn.Write([]byte("Message accepted"))
		if err != nil {
			break
		}
	}

	log.Println(err)
}
