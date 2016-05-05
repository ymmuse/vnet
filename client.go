package vnet

import (
	"encoding/gob"
	"errors"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

const (
	backendDialTimeout       = 5
	backendConnectChanMaxBuf = 1000
)

var (
	backendrw            sync.RWMutex
	tryConnToBackendChan = make(chan []byte, backendConnectChanMaxBuf)
	clientConnTable      = make(map[string]*clientConn)
	backendConnTimeTable = make(map[string]int64)

	errBackendConn      = errors.New("waiting to connect")
	errNotSupportNetMod = errors.New("not support net mode")
)

// Dial connects to the address on the named network. Now, The network only support "tcp".
// Examples:
// 	Dial("tcp", "12.34.56.78:80")
// 	Dial("tcp", ":80")
func Dial(net, address string) (conn net.Conn, err error) {
	if net != "tcp" {
		err = errNotSupportNetMod
		return
	}

	var sendtochan bool
	for try := 0; try < 3; try++ {
		backendrw.RLock()
		cc, ok := clientConnTable[address]
		backendrw.RUnlock()

		if !ok {
			if !sendtochan {
				sendtochan = true
				go func() { tryConnToBackendChan <- []byte(address) }()
			}
			err = errBackendConn

			time.Sleep(time.Millisecond * 200)
			continue
		}
		err = nil
		id := cc.newVConnID()
		conn = cc.newVConn(id)
		break
	}
	return
}

func init() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered in", r, ":", string(debug.Stack()))
			}
		}()

		for v := range tryConnToBackendChan {
			now := time.Now().Unix()
			ip := string(v)
			backendrw.RLock()
			_, ok := clientConnTable[ip]
			lastconntime := backendConnTimeTable[ip]
			backendrw.RUnlock()
			if ok {
				continue
			}

			// prevent cpu overload
			if now-lastconntime >= 1 {
				b := dialBackend(ip)
				if b != nil {
					backendrw.Lock()
					clientConnTable[ip] = b
					backendConnTimeTable[ip] = now
					backendrw.Unlock()
				}
			}
		}

		log.Println("connToBackendWorker exit")
	}()
}

func dialBackend(address string) (c *clientConn) {
	log.Println("try to connect to backend", address)
	conn, err := net.DialTimeout("tcp", address, time.Second*time.Duration(backendDialTimeout))
	if err != nil {
		log.Printf("connect to %s error. %v\n", address, err)
		return nil
	}

	c = &clientConn{
		_vconn: &_vconn{
			native:     conn,
			encoder:    gob.NewEncoder(conn),
			decoder:    gob.NewDecoder(conn),
			vconnTable: make(map[uint64]*vconn),
		},
	}

	go func() {
		defer func() {
			conn.Close()
			if err := recover(); err != nil {
				log.Printf("connection backend address: %s %v\n", address, err)
			}
		}()

		c.loop()

		backendrw.Lock()
		delete(clientConnTable, address)
		backendrw.Unlock()

		log.Printf("backend %s disconnect. perpare reconnect\n", address)
		// reconnect signal
		tryConnToBackendChan <- []byte(address)
	}()

	return
}

type clientConn struct {
	*_vconn
}

func (c *clientConn) loop() {
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

		vc := c.getVConn(packet.ID)
		if vc == nil {
			log.Println("never execute here")
			break
		}

		if packet.Flag == _FlagConnClose {
			go func(vc *vconn, id uint64) {
				c.delVConn(id)
				vc.Close()
			}(vc, packet.ID)
			continue
		}

		err := vc.writeRecvbuf(packet.Playload)
		if err != nil {
			log.Println("client writeRecvbuf err:", err)
			go func(vc *vconn, id uint64) {
				c.closeRemoteVConn(id)
				c.delVConn(id)
				vc.Close()
			}(vc, packet.ID)
		}
	}

	c.cleanupVConn()
}
