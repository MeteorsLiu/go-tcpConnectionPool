package pool

import (
	"encoding/binary"
	"math/rand"
	"testing"
	"time"
)

func TestNetconn(t *testing.T) {
	p, err := New("127.0.0.1:9998", DefaultOpts())
	if err != nil {
		t.Errorf("cannot start")
		return
	}
	w := Wrapper(p)
	defer w.Close()
	id := make(chan int)

	for i := 0; i < 500; i++ {
		go func() {
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(rand.Int()))
			if _, err := w.Write(b); err != nil {
				t.Log(err)
			}
		}()
	}
	<-time.After(time.Minute)
}
