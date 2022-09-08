package connpool

import (
	"fmt"
	"io"
	"net"
	"sync"
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
	var wg sync.WaitGroup
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Write([]byte(fmt.Sprintf("Test%d", i)))
		}()
	}
	wg.Wait()
}
