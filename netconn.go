package pool

import (
	"net"
	"strconv"
	"time"
)

type PoolNetconn struct {
	pl *Pool
	cn *ConnNode
}

// DON'T USE single cn, NO EVERY CONNECTION is ready to read.
// use readableQueue to read packet.
func (p *PoolNetconn) Read(b []byte) (n int, err error) {
	c, err := p.pl.GetReadableConn()
	if err != nil {
		return
	}
	n, err = c.Read(b)
	return
}

func (p *PoolNetconn) Write(b []byte) (n int, err error) {
	c, err := p.pl.Get()
	if err != nil {
		return
	}
	defer p.pl.Put(c)
	p.cn = c
	n, err = c.Conn.Write(b)
	return
}

func (p *PoolNetconn) Close() error {
	p.pl.Close()
	return nil
}

func (p *PoolNetconn) LocalAddr() net.Addr {
	return nil
}
func (p *PoolNetconn) RemoteAddr() net.Addr {
	host, port, _ := net.SplitHostPort(p.pl.Remote())
	pt, _ := strconv.Atoi(port)
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: pt,
	}
}

func (p *PoolNetconn) SetDeadline(t time.Time) error {
	if p.cn != nil {
		return p.cn.Conn.SetDeadline(t)
	}
	return nil
}

func (p *PoolNetconn) SetReadDeadline(t time.Time) error {
	if p.cn != nil {
		p.cn.Conn.SetReadDeadline(t)
	}
	return nil
}

func (p *PoolNetconn) SetWriteDeadline(t time.Time) error {
	if p.cn != nil {
		p.cn.Conn.SetWriteDeadline(t)
	}
	return nil
}

// for those lazy people like me
func CreatePool(remote string) (net.Conn, error) {
	p, err := New(remote, DefaultOpts())
	if err != nil {
		return nil, err
	}
	return &PoolNetconn{
		pl: p,
	}, nil
}

func NetConn(p *Pool) net.Conn {
	return &PoolNetconn{
		pl: p,
	}
}
