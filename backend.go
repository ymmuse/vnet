package vnet

import (
	"encoding/gob"
	"net"
	"sync"
	"sync/atomic"
)

const (
	_FlagConnPing  = uint8(1)
	_FlagConnClose = uint8(1) << 1
)

type playloadPacket struct {
	ID       uint64
	Flag     uint8
	Playload []byte
}

type backend struct {
	native net.Conn

	pconnIncID uint64
	pconnrw    sync.RWMutex
	pconnTable map[uint64]*persistentConn

	decoder *gob.Decoder
	encoder *gob.Encoder
}

func (bk *backend) pconnWriter(id uint64, pc *persistentConn) {
	defer pc.Close()
	buf := make([]byte, 4096)

	for {
		n, err := pc.readWritebuf(buf)
		if err != nil {
			break
		}
		packet := playloadPacket{ID: id, Playload: buf[:n]}
		if err = bk.encoder.Encode(packet); err != nil {
			break
		}
	}
}

func (bk *backend) newPconnID() uint64 {
	return atomic.AddUint64(&bk.pconnIncID, 1)
}

func (bk *backend) addPConn(id uint64, pc *persistentConn) {
	bk.pconnrw.Lock()
	defer bk.pconnrw.Unlock()
	bk.pconnTable[id] = pc
}

func (bk *backend) getPConn(id uint64) *persistentConn {
	bk.pconnrw.RLock()
	defer bk.pconnrw.RUnlock()
	return bk.pconnTable[id]
}

func (bk *backend) delPConn(id uint64) {
	bk.pconnrw.Lock()
	defer bk.pconnrw.Unlock()
	delete(bk.pconnTable, id)
}

func (bk *backend) cleanupPConn() {
	bk.pconnrw.Lock()
	defer bk.pconnrw.Unlock()

	for id, pconn := range bk.pconnTable {
		delete(bk.pconnTable, id)
		pconn.Close()
	}
}

func (bk *backend) closeRemotePersistentConn(id uint64) error {
	packet := playloadPacket{
		ID:   id,
		Flag: _FlagConnClose,
	}
	return bk.encoder.Encode(&packet)
}

func (bk *backend) ping() error {
	packet := playloadPacket{
		Flag: _FlagConnPing,
	}
	return bk.encoder.Encode(&packet)
}
