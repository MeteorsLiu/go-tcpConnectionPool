package connpool

import (
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
			if _, err := w.Write([]byte(fmt.Sprintf("Test%d", i))); err != nil {
				t.Log(err)
			}
		}()
		t.Log(p.Len())
	}
	wg.Wait()
}
