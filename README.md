

### What is this?
vnet can establish virtual connections of client based on a native tcp connection from the front server.  


### Example

The Dial function connects to a server:
```golang
import "vnet"

conn, err := vnet.DialTCP("google.com:80")
if err != nil {
	// handle error
}

// ...
```

The Listen function creates servers:
```golang
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
# cd this.repo.dir
# go test -bench .
```