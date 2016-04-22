package vnet

import (
	"bytes"
	"log"
	"net"
	"sync"
	"time"
)

type client struct {
	*backend
}

type backendConn struct {
	*backend
}

func (c *client) newPersistentConn() (pc *persistentConn) {
	pc = &persistentConn{
		recvbuf:  bytes.NewBuffer(make([]byte, 0, clientConnReadBuf)),
		recvcond: sync.NewCond(&sync.Mutex{}),

		writebuf:  bytes.NewBuffer(make([]byte, 0, clientConnWriteBuf)),
		writecond: sync.NewCond(&sync.Mutex{}),
	}

	id := c.newPconnID()
	c.addPConn(id, pc)
	go c.pconnWriter(id, pc)
	return
}

func (c *client) loop() {
	go func() {
		for {
			if err := c.ping(); err != nil {
				break
			}
			time.Sleep(3 * time.Second)
		}
	}()

	for {
		packet := playloadPacket{}
		if err := c.decoder.Decode(&packet); err != nil {
			break
		}

		pconn := c.getPConn(packet.ID)
		if pconn == nil {
			log.Println("never execute here")
			break
		}

		if packet.Flag == _FlagConnClose {
			go func(pconn *persistentConn, id uint64) {
				c.delPConn(id)
				pconn.Close()
			}(pconn, packet.ID)
			continue
		}

		err := pconn.writeRecvbuf(packet.Playload)
		if err != nil {
			log.Println("client writeRecvbuf err:", err)
			go func(pconn *persistentConn, id uint64) {
				c.closeRemotePersistentConn(id)
				c.delPConn(id)
				pconn.Close()
			}(pconn, packet.ID)
		}
	}

	c.cleanupPConn()
}

func (s *backendConn) newPersistentConn(id uint64) (pc *persistentConn) {
	pc = &persistentConn{
		recvbuf:  bytes.NewBuffer(make([]byte, 0, backendConnReadBuf)),
		recvcond: sync.NewCond(&sync.Mutex{}),

		writebuf:  bytes.NewBuffer(make([]byte, 0, backendConnWriteBuf)),
		writecond: sync.NewCond(&sync.Mutex{}),
	}

	s.addPConn(id, pc)
	go s.pconnWriter(id, pc)
	return
}

func (s *backendConn) loop(ch chan net.Conn) {
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

		pconn := s.getPConn(packet.ID)
		if packet.Flag == _FlagConnClose {
			if pconn != nil {
				go func(pconn *persistentConn, id uint64) {
					s.delPConn(id)
					pconn.Close()
				}(pconn, packet.ID)
			}
			continue
		}

		if pconn == nil {
			pconn = s.newPersistentConn(packet.ID)
			go func(c net.Conn) { ch <- c }(pconn)
		}

		err := pconn.writeRecvbuf(packet.Playload)
		if err != nil {
			log.Println("backendConn writeRecvbuf err:", err)
			go func(pconn *persistentConn, id uint64) {
				s.closeRemotePersistentConn(id)
				s.delPConn(id)
				pconn.Close()
			}(pconn, packet.ID)
		}
	}

	s.cleanupPConn()
}
