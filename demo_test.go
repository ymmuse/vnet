package main

import (
	"bytes"
	"demo/vnet"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())
	// start echo server
	go main()

	time.Sleep(time.Second)
	os.Exit(m.Run())
}

func doReqeust() {
	var conn net.Conn
	var err error
	for i := 0; i < 3; i++ {
		conn, err = vnet.DialTCP(srvaddr)
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
