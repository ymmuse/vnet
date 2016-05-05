package vnet

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"
)

var (
	srvaddr = "127.0.0.1:12345"
)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())

	go testsrv()

	time.Sleep(time.Second)
	os.Exit(m.Run())
}

func TestRequest(t *testing.T) {
	doReqeust()
}

func BenchmarkRequest(b *testing.B) {
	for i := 0; i < b.N; i++ {
		doReqeust()
	}
}

func BenchmarkRequestParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doReqeust()
		}
	})
}

func doReqeust() {
	var conn net.Conn
	var err error
	for i := 0; i < 3; i++ {
		conn, err = Dial("tcp", srvaddr)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		break
	}
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second * 10))

	n := rand.Int()%2048 + 10

	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = byte(rand.Int())
	}

	n0, err := conn.Write(out)
	if err != nil {
		panic(fmt.Errorf("test conn write err: %v", err))
	}

	rcv := make([]byte, n0)
	n1, err := io.ReadFull(conn, rcv)
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(out[:n0], rcv[:n1]) {
		fmt.Println("out: ", n0, " in:", n1, out, rcv)

		fmt.Println("out: ", hex.EncodeToString(out), "in:", hex.EncodeToString(rcv))
		panic(errors.New("echo server reply is not match"))
	}
}

func testsrv() {
	l, err := Listen("tcp", srvaddr)
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
