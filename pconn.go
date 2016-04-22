package vnet

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type persistentConn struct {
	recvbuf    *bytes.Buffer
	recvcond   *sync.Cond
	recvbufmtx sync.Mutex

	writebuf    *bytes.Buffer
	writecond   *sync.Cond
	writebufmtx sync.Mutex

	closed int32
}

func (c *persistentConn) Read(b []byte) (n int, err error) {
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

func (c *persistentConn) Write(b []byte) (n int, err error) {
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

func (c *persistentConn) Close() error {
	atomic.StoreInt32(&c.closed, 1)
	c.writecond.Broadcast()
	c.recvcond.Broadcast()
	return nil
}

func (c *persistentConn) LocalAddr() net.Addr {
	return nil
}

func (c *persistentConn) RemoteAddr() net.Addr {
	return nil
}

func (c *persistentConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *persistentConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *persistentConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *persistentConn) writeRecvbuf(b []byte) error {
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

func (c *persistentConn) readWritebuf(b []byte) (n int, err error) {
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
