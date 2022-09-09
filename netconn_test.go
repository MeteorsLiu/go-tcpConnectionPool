package connpool

import (
	"encoding/binary"
	"sync"
	"testing"
)

func TestNetconn(t *testing.T) {
	var wg sync.WaitGroup
	p := New("127.0.0.1:9998", DefaultOpts())
	w := Wrapper(p)
	defer w.Close()
	id := make(chan int)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(<-id))
			if _, err := w.Write(b); err != nil {
				t.Log(err)
			}
		}()
		id <- i
		t.Log(p.Len())
	}
	wg.Wait()
}
