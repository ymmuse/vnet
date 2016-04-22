package vnet

import (
	"encoding/gob"
	"log"
	"net"
	"time"
)

type backendServer struct {
	native      net.Listener
	newconnChan chan net.Conn
}

func Listen(lnet, laddr string) (net.Listener, error) {
	var bs *backendServer
	l, err := net.Listen(lnet, laddr)
	if err == nil {
		bs = &backendServer{
			native:      l,
			newconnChan: make(chan net.Conn, serverAcceptChanMaxBuf),
		}

		go bs.nativeAccept()
	}

	return bs, err
}

func (bs *backendServer) Accept() (c net.Conn, err error) {
	c = <-bs.newconnChan
	return
}

func (bs *backendServer) Close() error {
	return bs.native.Close()
}

func (bs *backendServer) Addr() net.Addr {
	return bs.native.Addr()
}

func (bs *backendServer) nativeAccept() {
	var tempDelay time.Duration
	for {
		conn, err := bs.native.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			log.Fatal(err)
		}
		tempDelay = 0

		go func(c net.Conn) {
			defer c.Close()
			bc := &backendConn{
				backend: &backend{
					native:     c,
					encoder:    gob.NewEncoder(c),
					decoder:    gob.NewDecoder(c),
					pconnTable: make(map[uint64]*persistentConn),
				},
			}
			bc.loop(bs.newconnChan)
		}(conn)
	}
}
