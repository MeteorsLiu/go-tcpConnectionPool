package connpool

import (
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
)

func TestNetconn(t *testing.T) {
	var wg sync.WaitGroup
	p := New("127.0.0.1:9998", DefaultOpts())
	w := Wrapper(p)

	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(i))
			if _, err := w.Write(b); err != nil {
				t.Log(err)
			}
		}()
		t.Log(p.Len())
	}
	wg.Wait()
}
