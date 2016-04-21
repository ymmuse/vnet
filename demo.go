package main

import (
	"demo/vnet"
	"io"
	"log"
	"net"
	"os"
)

var (
	srvaddr = "127.0.0.1:12345"
)

func main() {
	l, err := vnet.Listen("tcp", srvaddr)
	if err != nil {
		log.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	defer l.Close()
	log.Println("Listening on " + srvaddr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}

		go func(c net.Conn) {
			defer c.Close()
			_, err := io.Copy(c, c)
			switch err {
			case io.EOF:
				err = nil
				return
			case nil:
				return
			}
			panic(err)
		}(c)
	}
}
