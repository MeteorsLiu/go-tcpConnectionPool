package pool

import (
	"io"
	"net"
)

type PoolWrapper struct {
	pl *Pool
}

func (p *PoolWrapper) Read(b []byte) (n int, err error) {
	c, err := p.pl.GetReadableConn()
	if err != nil {
		return
	}
	n, err = c.Read(b)
	return
}

func (p *PoolWrapper) Write(b []byte) (n int, err error) {
	c, err := p.pl.Get()
	if err != nil {
		return
	}
	defer p.pl.Put(c)
	n, err = c.Conn.Write(b)
	return
}

func (p *PoolWrapper) ReadFrom(r io.Reader) (n int64, err error) {
	c, err := p.pl.Get()
	if err != nil {
		return
	}
	defer p.pl.Put(c)
	n, err = c.Conn.(*net.TCPConn).ReadFrom(r)
	return
}

func (p *PoolWrapper) Close() error {
	p.pl.Close()
	return nil
}

func Wrapper(p *Pool) io.ReadWriteCloser {
	return &PoolWrapper{p}
}
