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

var (
	backendrw            sync.RWMutex
	tryConnToBackendChan = make(chan []byte, backendConnectChanMaxBuf)
	backendTable         = make(map[string]*client)
	backendConnTimeTable = make(map[string]int64)

	backendConnErr = errors.New("waiting to connect")
)

func DialTCP(address string) (conn net.Conn, err error) {
	backendrw.RLock()
	bkc, ok := backendTable[address]
	backendrw.RUnlock()

	if !ok {
		go func() { tryConnToBackendChan <- []byte(address) }()
		err = backendConnErr
		return
	}

	conn = bkc.newPersistentConn()
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
			_, ok := backendTable[ip]
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
					backendTable[ip] = b
					backendConnTimeTable[ip] = now
					backendrw.Unlock()
				}
			}
		}

		log.Println("connToBackendWorker exit")
	}()
}

func dialBackend(address string) (c *client) {
	log.Println("try to connect to backend", address)
	conn, err := net.DialTimeout("tcp", address, time.Second*time.Duration(backendDialTimeout))
	if err != nil {
		log.Printf("connect to %s error. %v\n", address, err)
		return nil
	}

	c = &client{
		backend: &backend{
			native:     conn,
			encoder:    gob.NewEncoder(conn),
			decoder:    gob.NewDecoder(conn),
			pconnTable: make(map[uint64]*persistentConn),
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
		delete(backendTable, address)
		backendrw.Unlock()

		log.Printf("backend %s disconnect. perpare reconnect\n", address)
		// reconnect signal
		tryConnToBackendChan <- []byte(address)
	}()

	return
}
