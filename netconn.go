package connpool

import (
	"net"
	"time"
)

type Conn struct {
	p *Pool
}

func (cn *Conn) Read(b []byte) (n int, err error) {
	c, err := cn.p.Get()
	if err != nil {
		return
	}
	n, err = c.Read(b)
	// Only put the fine connection into the pool
	if err == nil {
		cn.p.Put(c)
	} else {
		c.Close()
	}
	return
}

func (cn *Conn) Write(b []byte) (n int, err error) {
	c, err := cn.p.Get()
	if err != nil {
		return
	}
	n, err = c.Write(b)
	if err == nil {
		cn.p.Put(c)
	} else {
		c.Close()
	}
	return
}

func (cn *Conn) Close() error {
	cn.p.Close()
	return nil
}

func (cn *Conn) LocalAddr() net.Addr {
	return nil
}

func (cn *Conn) RemoteAddr() net.Addr {
	return nil
}

func (cn *Conn) SetDeadline(t time.Time) error {
	cn.p.SetDeadline(t)
	return nil
}

func (cn *Conn) SetReadDeadline(t time.Time) error {
	cn.p.SetReadDeadline(t)
	return nil
}

func (cn *Conn) SetWriteDeadline(t time.Time) error {
	cn.p.SetWriteDeadline(t)
	return nil
}
func Wrapper(p *Pool) net.Conn {
	return &Conn{p}
}
