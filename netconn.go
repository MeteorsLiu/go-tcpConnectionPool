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
	defer cn.p.Put(c)
	n, err = c.Read(b)
	if err == nil {
		defer cn.p.Put(c)
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
		defer cn.p.Put(c)
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

func (cn *Conn) SetDeadline(_ time.Time) error {
	return nil
}

func (cn *Conn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (cn *Conn) SetWriteDeadline(_ time.Time) error {
	return nil
}
func Wrapper(p *Pool) net.Conn {
	return &Conn{p}
}
