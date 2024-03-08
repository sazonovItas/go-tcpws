package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	gotcpws "github.com/sazonovItas/go-tcpws"
)

var portFlag = flag.String("p", "8080", "port to listen")

func main() {
	flag.Parse()

	c, err := net.Dial("tcp", ":"+*portFlag)
	if err != nil {
		panic(err)
	}
	log.Println("new connection on addr:", c.RemoteAddr())

	conn := gotcpws.NewFrameConnection(c, nil, nil, 0)
	defer conn.Close()

	rd := bufio.NewReader(os.Stdout)
	go func() {
		for {
			msg, err := conn.ReadFrame()
			if err != nil {
				return
			}

			fmt.Println(string(msg))
		}
	}()

	for {
		msg, _, err := rd.ReadLine()
		if err != nil {
			continue
		}

		_, err = conn.Write(msg)
		if err != nil {
			break
		}
	}
}
