package pool

import (
	"encoding/binary"
	"sync"
	"testing"
)

func TestNetconn(t *testing.T) {
	var wg sync.WaitGroup
	p, err := New("127.0.0.1:9998", DefaultOpts())
	if err != nil {
		t.Errorf("cannot start")
		return
	}
	w := Wrapper(p)
	defer w.Close()
	id := make(chan int)
	wg.Add(500)
	for i := 0; i < 500; i++ {
		go func() {
			defer wg.Done()
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(<-id))
			if _, err := w.Write(b); err != nil {
				t.Log(err)
			}
		}()
		id <- i
	}
	wg.Wait()
}
