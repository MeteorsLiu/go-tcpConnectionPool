package pool

import (
	"encoding/binary"
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
	go func() {
		b := make([]byte, 2)
		for {
			_, err := w.Read(b)
			if err != nil {
				t.Log(err)
				return
			}
			t.Log(binary.LittleEndian.Uint16(b))
		}
	}()
	for i := 0; i < 500; i++ {
		go func() {
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(1))
			if _, err := w.Write(b); err != nil {
				t.Log(err)
			}
		}()
	}
	<-time.After(time.Minute)
}
