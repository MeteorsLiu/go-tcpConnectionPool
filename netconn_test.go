package connpool

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
)

func serve(wg *sync.WaitGroup, t *testing.T) {
	defer wg.Done()
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
	var wg sync.WaitGroup
	wg.Add(1)
	go serve(&wg, t)
	w := Wrapper(New("127.0.0.1:9999", DefaultOpts()))

	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := w.Write([]byte(fmt.Sprintf("Test%d", i))); err != nil {
				t.Log(err)
			}
		}()
	}
	wg.Wait()
}
