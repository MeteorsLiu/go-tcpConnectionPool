package pool

import (
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// THIS IS NOT THREAD-SAFE
// How to use? this is for one goroutine to wrapper the pool but which only requires one connection.
type PoolNetconn struct {
	pl   *Pool
	cn   *ConnNode
	cond *sync.Cond
}

func (p *PoolNetconn) getConn() (err error) {
	p.cond.L.Lock()
	p.cn, err = p.pl.Get()
	p.cond.L.Unlock()
	p.cond.Broadcast()
	return err
}
func (p *PoolNetconn) Read(b []byte) (n int, err error) {
	if p.cn == nil || p.cn.IsDown() || p.cn.IsClosed() {
		p.cond.L.Lock()
		p.cond.Wait()
		p.cond.L.Unlock()
	}
	if p.cn == nil {
		err = NO_AVAILABLE_CONN
		return
	}
	n, err = p.cn.Conn.Read(b)
	return
}

func (p *PoolNetconn) ReadFrom(r io.Reader) (n int64, err error) {
	if p.cn == nil || p.cn.IsDown() || p.cn.IsClosed() {
		if err = p.getConn(); err != nil {
			return
		}
	} else {
		// someone grabs the connection
		// try to regrab one
		if !p.cn.IsAvailable() {
			if err = p.getConn(); err != nil {
				return
			}
		}
	}
	defer p.cn.Lock.Unlock()
	n, err = p.cn.Conn.(*net.TCPConn).ReadFrom(r)
	return
}

func (p *PoolNetconn) WriteTo(w io.Writer) (n int64, err error) {
	if p.cn == nil || p.cn.IsDown() || p.cn.IsClosed() {
		p.cond.L.Lock()
		p.cond.Wait()
		p.cond.L.Unlock()
	}
	if p.cn == nil {
		err = NO_AVAILABLE_CONN
		return
	}
	n, err = io.Copy(w, p.cn.Conn)
	return
}

// this function will keep the same one connection unless the writing function returns an error.
// In the most case, it only requires one connection.
func (p *PoolNetconn) Write(b []byte) (n int, err error) {
	if p.cn == nil || p.cn.IsDown() || p.cn.IsClosed() {
		if err = p.getConn(); err != nil {
			return
		}
	} else {
		// someone grabs the connection
		// try to regrab one
		if !p.cn.IsAvailable() {
			if err = p.getConn(); err != nil {
				return
			}
		}
	}
	// release the lock after using.
	// so that the pool could maintain this connection when it is down.
	defer p.cn.Lock.Unlock()
	n, err = p.cn.Conn.Write(b)
	return
}

func (p *PoolNetconn) Close() error {
	p.pl.Close()
	return nil
}

func (p *PoolNetconn) LocalAddr() net.Addr {
	if p.cn != nil {
		return p.cn.Conn.LocalAddr()
	}
	return nil
}
func (p *PoolNetconn) RemoteAddr() net.Addr {
	if p.cn != nil {
		return p.cn.Conn.RemoteAddr()
	}
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
		pl:   p,
		cond: sync.NewCond(&sync.Mutex{}),
	}, nil
}

func NetConn(p *Pool) net.Conn {
	return &PoolNetconn{
		pl:   p,
		cond: sync.NewCond(&sync.Mutex{}),
	}
}
