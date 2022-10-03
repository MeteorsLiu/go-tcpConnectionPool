package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

const (
	MIN_SIZE = 128
)

var (
	NO_AVAILABLE_CONN = errors.New("no available connections!")
)

type ConnNode struct {
	Conn     net.Conn
	consumer int32
	Lock     sync.Mutex
	prev     *ConnNode
	next     *ConnNode
}
type Pool struct {
	head        *ConnNode
	tail        *ConnNode
	mutex       sync.RWMutex
	num         int32
	remote      string
	maxSize     int
	minConsumer int32
	connContext context.Context
	dialer      *net.Dialer
}

func (p *Pool) MoveToHead(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if cp.prev != nil {
		cp.prev.next = cp.next
	}
	if cp.next != nil {
		cp.next.prev = cp.prev
	}
	cp.next = p.head
	cp.prev = nil
	p.head.prev = cp
	p.head = cp
}

func (p *Pool) MoveToTail(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if cp.prev != nil {
		cp.prev.next = cp.next
	}
	if cp.next != nil {
		cp.next.prev = cp.prev
	}
	cp.next = nil
	cp.prev = p.tail
	p.tail.next = cp
	p.tail = cp
}

func (p *Pool) Get() (*ConnNode, error) {
	p.mutex.RLock()
	node := p.head.next
	p.mutex.RUnlock()
	succ := false
	minCount := int32(0)
	var min *ConnNode
	for !succ && node != nil {
		// try to grab the lock
		succ = node.Lock.TryLock()
		if !succ {
			if minCount > node.consumer || minCount == 0 {
				minCount = node.consumer
				min = node
			}
			p.mutex.RLock()
			node = node.next
			p.mutex.RUnlock()
		}
	}
	if node == nil {
		// tries are all fail
		// stage into the lock strvation
		if min != nil {
			// grab the lock
			min.Lock.Lock()
			node = min
		} else {
			return nil, NO_AVAILABLE_CONN
		}
	}
	minC := atomic.AddInt32(&node.consumer, 1)
	if atomic.LoadInt32(&p.minConsumer) > minC {
		_ = atomic.SwapInt32(&p.minConsumer, minC)
	}
	return node, nil
}
func (p *Pool) Put(c *ConnNode) {
	consumer := atomic.AddInt32(&c.consumer, -1)
	if atomic.LoadInt32(&p.minConsumer) > consumer {
		_ = atomic.SwapInt32(&p.minConsumer, consumer)
		p.MoveToHead(c)
	} else {
		p.MoveToTail(c)
	}

}

func (p *Pool) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	node := p.head.next
	for node != nil {
		// close the connection
		// if there is someone reading or writing, it will return EOF immediately.
		node.Conn.Close()
		node.Lock.Unlock()
		tmp := node.next
		// tell gc to free it
		// actually it isn't required
		node = nil
		node = tmp
	}
}
func (p *Pool) Push(c net.Conn) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	new := &ConnNode{
		Conn: c,
	}
	if p.head == nil && p.tail == nil {
		p.head = new
		p.tail = new
	} else {
		new.prev = p.tail
		p.tail.next = new
		p.tail = new
	}

}

// dialOne creates a TCP Connection,
func (p *Pool) dialOne() (net.Conn, error) {
	var c net.Conn
	var err error
	if p.connContext == nil {
		c, err = p.dialer.Dial("tcp", p.remote)
	} else {
		c, err = p.dialer.DialContext(p.connContext, "tcp", p.remote)
	}
	return c, err

}
func (p *Pool) connInit(minSize int32) error {
	for i := int32(0); i < minSize; i++ {
		c, err := p.dialOne()
		if err != nil {
			return err
		}
		p.Push(c)
	}

}
func New(remote string, opts Opts) (*Pool, error) {
	var d *net.Dialer
	var m int32
	var c context.Context
	if len(opts) > 0 {
		d, m, c = opts.Parse()
	}

	if d == nil {
		d = &net.Dialer{}
	}
	if m == 0 {
		m = MIN_SIZE
	}
	p := &Pool{remote: remote, connContext: c, dialer: d}
	if err := p.connInit(m); err != nil {
		return nil, err
	}
	return p, nil
}
