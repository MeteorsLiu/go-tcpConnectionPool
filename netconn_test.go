package connpool

import (
	"fmt"
	"sync"
	"testing"
)

func TestNetconn(t *testing.T) {
	var wg sync.WaitGroup

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
