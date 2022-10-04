package pool

import "io"

type PoolWrapper struct {
	pl *Pool
}

func (p *PoolWrapper) Read(b []byte) (n int, err error) {
	c, err := p.pl.GetReadableConn()
	if err != nil {
		return
	}
	return c.Read(b)
}

func (p *PoolWrapper) Write(b []byte) (n int, err error) {
	c, err := p.pl.Get()
	if err != nil {
		return
	}
	defer p.pl.Put(c)
	return c.Write(b)
}

func (p *PoolWrapper) Close() error {
	p.pl.Close()
	return nil
}

func Wrapper(p *Pool) io.ReadWriteCloser {
	return &PoolWrapper{p}
}
