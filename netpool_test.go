package connpool

import (
	"fmt"
	"io"
	"net"
	"testing"
)

func serve(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:9999")
	for {
		c, err := l.Accept()
		if err != nil {
			t.Log(err)
			continue
		}
		go func() {
			defer c.Close()
			res, _ := io.ReadAll(c)
			fmt.Println(res)
		}()
	}
}

func TestNetconn(t *testing.T) {
	go serve(t)
	w := Wrapper(New("127.0.0.1:9999", DefaultOpts()))
	for i := 0; i < 10000; i++ {
		go w.Write([]byte(fmt.Sprintf("Test%d", i)))
	}
}
