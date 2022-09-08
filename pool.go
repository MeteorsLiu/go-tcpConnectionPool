package connpool

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"
)

const (
	MIN_SIZE = 1024
)

type Pool struct {
	minSize       int32
	maxSize       int32
	len           int32
	remoteAddr    string
	conn          chan net.Conn
	dialer        *net.Dialer
	timeout       time.Duration
	customContext context.Context
	close         context.Context
	Deadline      struct {
		deadLine      *time.Time
		readDeadline  *time.Time
		writeDeadline *time.Time
	}
	doClose context.CancelFunc
}

// Put the TCP connection into the pool.
func (p *Pool) Put(c net.Conn) {
	select {
	case p.conn <- c:
		_ = atomic.AddInt32(&p.len, 1)
	default:
		// don't put it into the pool if the pool is full
	}
}

// Get one TCP connection from the pool.
//
// If the connection reaches the max limit, poll until one enqueue during TIMEOUT.
//
// If not, set up a new one
func (p *Pool) Get() (net.Conn, error) {
	var c net.Conn
	var err error
	defer func() {
		if err == nil {
			_ = atomic.AddInt32(&p.len, -1)
		}
	}()
	select {
	case c = <-p.conn:
	default:
		// no available connections in the pool
		// so try to get one
		if p.len >= p.maxSize {
			// wait until available
			ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
			defer cancel()
			for {
				select {
				case c = <-p.conn:
					return c, nil
				case <-ctx.Done():
					return p.dialOne()
				case <-p.close.Done():
					return nil, errors.New("the connections pool is closed")
				}
			}
		} else {
			c, err = p.dialOne()
		}

	}
	return c, err
}

// Flush dequeues all TCP Connections and close them.
func (p *Pool) Flush() {
	for {
		select {
		case c := <-p.conn:
			c.Close()
			_ = atomic.AddInt32(&p.len, -1)
		default:
			if p.len == 0 {
				return
			}
		}
	}
}
func (p *Pool) Close() {
	p.doClose()
	p.Flush()
}

func (p *Pool) SetDeadline(t time.Time) {
	p.Deadline.deadLine = &t
}
func (p *Pool) SetReadDeadline(t time.Time) {
	p.Deadline.readDeadline = &t
}
func (p *Pool) SetWriteDeadline(t time.Time) {
	p.Deadline.writeDeadline = &t
}
func (p *Pool) setDeadline(c net.Conn) {
	if p.Deadline.deadLine != nil {
		c.SetDeadline(*p.Deadline.deadLine)
		return
	}
	if p.Deadline.readDeadline != nil {
		c.SetReadDeadline(*p.Deadline.readDeadline)
		return
	}
	if p.Deadline.writeDeadline != nil {
		c.SetWriteDeadline(*p.Deadline.writeDeadline)
		return
	}

}

// dialOne creates a TCP Connection,
func (p *Pool) dialOne() (net.Conn, error) {
	var c net.Conn
	var err error
	if p.customContext == nil {
		c, err = p.dialer.Dial("tcp", p.remoteAddr)
	} else {
		c, err = p.dialer.DialContext(p.customContext, "tcp", p.remoteAddr)
	}
	if err == nil {
		p.setDeadline(c)
	}
	return c, err

}

func New(remoteAddr string, opts Opts) *Pool {
	var d *net.Dialer
	var t time.Duration
	var m int32
	var c context.Context
	if len(opts) > 0 {
		d, t, m, c = opts.Parse()
	}
	if t == 0 {
		t = 5 * time.Second
	}
	if d == nil {
		d = &net.Dialer{}
	}
	if m == 0 {
		m = MIN_SIZE
	}

	MAX_SIZE := GetSysMax()
	p := &Pool{
		dialer:        d,
		conn:          make(chan net.Conn, MAX_SIZE),
		maxSize:       MAX_SIZE,
		minSize:       m,
		timeout:       t,
		remoteAddr:    remoteAddr,
		customContext: c,
	}
	go func() {
		for i := int32(0); i < p.minSize; i++ {
			if c, err := p.dialOne(); err == nil {
				p.Put(c)
			}
		}
	}()
	p.close, p.doClose = context.WithCancel(context.Background())
	return p
}
