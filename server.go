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

// Listen announces on the local network address laddr.
// The network lnet must be a stream-oriented network: "tcp",
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
			bc := &serverConn{
				_vconn: &_vconn{
					native:     c,
					encoder:    gob.NewEncoder(c),
					decoder:    gob.NewDecoder(c),
					vconnTable: make(map[uint64]*vconn),
				},
			}
			bc.loop(bs.newconnChan)
		}(conn)
	}
}

type serverConn struct {
	*_vconn
}

func (s *serverConn) loop(ch chan net.Conn) {
	nowtime := time.Now()

	go func() {
		for {
			if time.Now().Sub(nowtime) > time.Second*10 {
				s.native.Close()
				break
			}
			time.Sleep(3 * time.Second)
		}
	}()

	for {
		packet := playloadPacket{}
		if err := s.decoder.Decode(&packet); err != nil {
			log.Println(err)
			break
		}

		if packet.Flag == _FlagConnPing {
			nowtime = time.Now()
			continue
		}

		vc := s.getVConn(packet.ID)
		if packet.Flag == _FlagConnClose {
			if vc != nil {
				go func(vc *vconn, id uint64) {
					s.delVConn(id)
					vc.Close()
				}(vc, packet.ID)
			}
			continue
		}

		if vc == nil {
			vc = s.newVConn(packet.ID)
			go func(c net.Conn) { ch <- c }(vc)
		}

		err := vc.writeRecvbuf(packet.Playload)
		if err != nil {
			log.Println("serverConn writeRecvbuf err:", err)
			go func(vc *vconn, id uint64) {
				s.closeRemoteVConn(id)
				s.delVConn(id)
				vc.Close()
			}(vc, packet.ID)
		}
	}

	s.cleanupVConn()
}
