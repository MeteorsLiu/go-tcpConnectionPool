package pool

import (
	"bufio"
	"encoding/binary"
	"io"
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
		count := 0
		r := bufio.NewReader(w)
		for {
			_, err := io.ReadFull(r, b)
			if err != nil {
				t.Log(err)
				return
			}
			bf := int(binary.LittleEndian.Uint16(b))
			if bf == 1 {
				count++
			}
			t.Log(count)
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
