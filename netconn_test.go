package connpool

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
)

func serve(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:9998")
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	for {
		c, err := l.Accept()
		if err != nil {
			t.Log(err)
			continue
		}
		go func() {
			defer c.Close()
			res, _ := io.ReadAll(c)
			t.Log(string(res))
		}()
	}
}

func TestNetconn(t *testing.T) {
	var wg sync.WaitGroup
	go serve(t)
	w := Wrapper(New("127.0.0.1:9998", DefaultOpts()))

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
