
### What is this?
vnet can establish virtual connections of client based on a native tcp connection from the front server.  


[![Build Status](https://travis-ci.org/ymmuse/vnet.svg?branch=master)](https://travis-ci.org/ymmuse/vnet)
[![GoDoc](https://godoc.org/github.com/ymmuse/vnet?status.svg)](https://godoc.org/github.com/ymmuse/vnet)


### Example

The Dial function connects to a server:
```go
import "vnet"

conn, err := vnet.DialTCP("google.com:80")
if err != nil {
	// handle error
}

// ...
```

The Listen function creates servers:
```go
import "vnet"

ln, err := vnet.Listen("tcp", ":8080")
if err != nil {
	// handle error
}
for {
	conn, err := ln.Accept()
	if err != nil {
		// handle error
	}
	go handleConnection(conn)
}
```


### Benchmark

```shell
cd this.repo.dir
go test -bench .
```
