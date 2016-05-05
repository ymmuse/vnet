package vnet

import (
	"bytes"
	"encoding/gob"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	vconnReadBuf           = 4096
	vconnWriteBuf          = 4096
	serverAcceptChanMaxBuf = 20000

	_FlagConnPing  = uint8(1)
	_FlagConnClose = uint8(1) << 1
)

type vconn struct {
	recvbuf    *bytes.Buffer
	recvcond   *sync.Cond
	recvbufmtx sync.Mutex

	writebuf    *bytes.Buffer
	writecond   *sync.Cond
	writebufmtx sync.Mutex

	closed int32
}

func (c *vconn) Read(b []byte) (n int, err error) {
	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			err = io.EOF
			break
		}

		c.recvcond.L.Lock()

		c.recvbufmtx.Lock()
		nn := c.recvbuf.Len()
		if nn > 0 {
			n, err = c.recvbuf.Read(b)
		}
		c.recvbufmtx.Unlock()

		if nn > 0 {
			c.recvcond.L.Unlock()
			break
		} else if atomic.LoadInt32(&c.closed) == 0 {
			c.recvcond.Wait()
		}
		c.recvcond.L.Unlock()
	}
	return
}

func (c *vconn) Write(b []byte) (n int, err error) {
	defer func() {
		c.writecond.L.Lock()
		c.writecond.Signal()
		c.writecond.L.Unlock()
	}()

	if atomic.LoadInt32(&c.closed) == 1 {
		err = io.EOF
		return
	}

	c.writebufmtx.Lock()
	n, err = c.writebuf.Write(b)
	c.writebufmtx.Unlock()
	return
}

func (c *vconn) Close() error {
	atomic.StoreInt32(&c.closed, 1)
	c.writecond.Broadcast()
	c.recvcond.Broadcast()
	return nil
}

func (c *vconn) LocalAddr() net.Addr {
	return nil
}

func (c *vconn) RemoteAddr() net.Addr {
	return nil
}

func (c *vconn) SetDeadline(t time.Time) error {
	return nil
}

func (c *vconn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *vconn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *vconn) writeRecvbuf(b []byte) error {
	defer func() {
		c.recvcond.L.Lock()
		c.recvcond.Signal()
		c.recvcond.L.Unlock()
	}()

	if atomic.LoadInt32(&c.closed) == 1 {
		return io.EOF
	}

	c.recvbufmtx.Lock()
	n, err := c.recvbuf.Write(b)
	c.recvbufmtx.Unlock()

	if n != len(b) {
		err = io.ErrShortWrite
	}

	return err
}

func (c *vconn) readWritebuf(b []byte) (n int, err error) {
	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			err = io.EOF
			break
		}

		c.writecond.L.Lock()

		c.writebufmtx.Lock()
		nn := c.writebuf.Len()
		if nn > 0 {
			n, err = c.writebuf.Read(b)
		}
		c.writebufmtx.Unlock()

		if nn > 0 {
			c.writecond.L.Unlock()
			break
		} else if atomic.LoadInt32(&c.closed) == 0 {
			c.writecond.Wait()
		}
		c.writecond.L.Unlock()
	}

	return
}

type playloadPacket struct {
	ID       uint64
	Flag     uint8
	Playload []byte
}

type _vconn struct {
	native net.Conn

	vconnIncID uint64
	vconnrw    sync.RWMutex
	vconnTable map[uint64]*vconn

	decoder *gob.Decoder
	encoder *gob.Encoder
}

func (v *_vconn) newVConn(id uint64) (vc *vconn) {
	vc = &vconn{
		recvbuf:  bytes.NewBuffer(make([]byte, 0, vconnReadBuf)),
		recvcond: sync.NewCond(&sync.Mutex{}),

		writebuf:  bytes.NewBuffer(make([]byte, 0, vconnWriteBuf)),
		writecond: sync.NewCond(&sync.Mutex{}),
	}

	v.addVConn(id, vc)
	go v.vconnWriter(id, vc)
	return
}

func (v *_vconn) vconnWriter(id uint64, vc *vconn) {
	defer vc.Close()
	buf := make([]byte, 4096)

	for {
		n, err := vc.readWritebuf(buf)
		if err != nil {
			break
		}
		packet := playloadPacket{ID: id, Playload: buf[:n]}
		if err = v.encoder.Encode(packet); err != nil {
			break
		}
	}
}

func (v *_vconn) newVConnID() uint64 {
	return atomic.AddUint64(&v.vconnIncID, 1)
}

func (v *_vconn) addVConn(id uint64, vc *vconn) {
	v.vconnrw.Lock()
	defer v.vconnrw.Unlock()
	v.vconnTable[id] = vc
}

func (v *_vconn) getVConn(id uint64) *vconn {
	v.vconnrw.RLock()
	defer v.vconnrw.RUnlock()
	return v.vconnTable[id]
}

func (v *_vconn) delVConn(id uint64) {
	v.vconnrw.Lock()
	defer v.vconnrw.Unlock()
	delete(v.vconnTable, id)
}

func (v *_vconn) cleanupVConn() {
	v.vconnrw.Lock()
	defer v.vconnrw.Unlock()

	for id, vconn := range v.vconnTable {
		delete(v.vconnTable, id)
		vconn.Close()
	}
}

func (v *_vconn) closeRemoteVConn(id uint64) error {
	packet := playloadPacket{
		ID:   id,
		Flag: _FlagConnClose,
	}
	return v.encoder.Encode(&packet)
}

func (v *_vconn) ping() error {
	packet := playloadPacket{
		Flag: _FlagConnPing,
	}
	return v.encoder.Encode(&packet)
}
