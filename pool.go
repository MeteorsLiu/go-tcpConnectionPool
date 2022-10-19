package pool

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	MIN_SIZE                = 16
	MIN_READABLE_QUEUE_SIZE = 1024
	ATTEMPT_RECONNECT       = 100
	RECONNECT_TIMEOUT       = 300
	// it seems that the value of syscall.EPOLLET is wrong
	EPOLLET = 0x80000000
)

var (
	NO_AVAILABLE_CONN = errors.New("no available connections!")
	POOL_CLOSED       = errors.New("pool has been closed")
)

type ConnNode struct {
	Conn     net.Conn
	consumer int32
	Lock     sync.Mutex
	prev     *ConnNode
	next     *ConnNode
	fd       int32
}
type Pool struct {
	head             *ConnNode
	tail             *ConnNode
	mutex            sync.RWMutex
	len              int
	remote           string
	minConsumer      int32
	maxSize          int
	reconnect        int
	reconnectTimeout time.Duration
	connContext      context.Context
	dialer           *net.Dialer
	isClose          context.Context
	close            context.CancelFunc
	readableQueue    chan net.Conn
	epoll            struct {
		events [MIN_SIZE]syscall.EpollEvent
		fd     int
	}
}

// Move the node between next and prev
func (cn *ConnNode) MoveTo(next *ConnNode, prev *ConnNode) {
	if next != nil {
		next.prev = cn
	}
	if prev != nil {
		prev.next = cn
	}
	if cn.prev != nil {
		cn.prev.next = cn.next
	}
	if cn.next != nil {
		cn.next.prev = cn.prev
	}
	cn.prev = prev
	cn.next = next
}

// move the node after n
func (cn *ConnNode) After(n *ConnNode) {
	if cn.prev != nil {
		cn.prev.next = cn.next
	}
	if cn.next != nil {
		cn.next.prev = cn.prev
	}
	n.next = cn
	cn.prev = n
	cn.next = nil
}

// move the node ahead n
func (cn *ConnNode) Before(n *ConnNode) {
	if cn.prev != nil {
		cn.prev.next = cn.next
	}
	if cn.next != nil {
		cn.next.prev = cn.prev
	}
	n.prev = cn
	cn.prev = nil
	cn.next = n
}

func (p *Pool) Reconnect(cn *ConnNode) {
	cn.Lock.Lock()
	defer cn.Lock.Unlock()
	timeout, cancel := context.WithTimeout(context.Background(), p.reconnectTimeout)
	defer cancel()
	var err error
	for i := 0; i < p.reconnect; i++ {
		if cn.Conn != nil {
			p.eventDel(cn.fd)
			cn.Conn.Close()
			cn.Conn = nil
		}
		cn.Conn, err = p.dialWith(timeout)
		if err == nil && cn.Conn != nil {
			f, _ := cn.Conn.(*net.TCPConn).File()
			fd := int32(f.Fd())
			cn.fd = fd
			p.eventAdd(fd)
			return
		}
	}
}

// move the node to the head
func (p *Pool) MoveToHead(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	cp.Before(p.head)
	p.head = cp
}

// move the node to the tail
func (p *Pool) MoveToTail(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	cp.After(p.tail)
	p.tail = cp
}

// add readable connections to the readableQueue
func (p *Pool) markReadable(n int) {
	p.mutex.RLock()
	node := p.head
	p.mutex.RUnlock()
	for node != nil {
		for i := 0; i < n; i++ {
			if p.epoll.events[i].Fd == node.fd {
				if p.epoll.events[i].Events&syscall.EPOLLERR ||
					p.epoll.events[i].Events&syscall.EPOLLHUP {
					go p.Reconnect(node)
				}
				if p.epoll.events[i].Events & syscall.EPOLLIN {
					select {
					case p.readableQueue <- node.Conn:
					default:
					}
				}
			}
		}
		p.mutex.RLock()
		node = node.next
		p.mutex.RUnlock()
	}
}

// get a readable connection from readable Queue.
// commonly, it's blocking. If there's no avaiblable connection, wait until avaiable.
func (p *Pool) GetReadableConn() (net.Conn, error) {
	var c net.Conn
	select {
	case c = <-p.readableQueue:
		return c, nil
	case <-p.isClose.Done():
		return nil, POOL_CLOSED
	}

}

// get a writable connection.
func (p *Pool) Get() (*ConnNode, error) {
	p.mutex.RLock()
	node := p.head
	p.mutex.RUnlock()
	succ := false
	minCount := atomic.LoadInt32(&p.minConsumer)
	var min *ConnNode
	for !succ && node != nil {
		// try to grab the lock.
		// trylock will not pause the goroutine.
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
			// grab the lock forcely.
			min.Lock.Lock()
			node = min
		} else {
			if p.len < p.maxSize {
				// if all are busy
				// try to dial a new one
				c, err := p.dialOne()
				if err != nil {
					return nil, NO_AVAILABLE_CONN
				}
				n := p.Push(c)
				n.Lock.Lock()
				node = n
			} else {
				// just wait
				p.mutex.RLock()
				node = p.head
				p.mutex.RUnlock()
				node.Lock.Lock()
			}
		}
	}
	minC := atomic.AddInt32(&node.consumer, 1)
	gminC := atomic.LoadInt32(&p.minConsumer)
	if gminC > minC || gminC == 0 {
		_ = atomic.SwapInt32(&p.minConsumer, minC)
	}
	return node, nil
}

// put the writable connection into the pool.
func (p *Pool) Put(c *ConnNode) {
	consumer := atomic.AddInt32(&c.consumer, -1)
	c.Lock.Unlock()
	if atomic.LoadInt32(&p.minConsumer) > consumer {
		_ = atomic.SwapInt32(&p.minConsumer, consumer)
		if p.head != c {
			p.MoveToHead(c)
		}
	} else {
		if p.tail != c {
			p.MoveToTail(c)
		}
	}

}

// close all connection.
func (p *Pool) Close() {
	p.EpollClose()
	node := p.head
	for node != nil {
		// close the connection
		// if there is someone reading or writing, it will return EOF immediately.
		node.Conn.Close()
		node = node.next
	}
}

// this is for creating a new connection.
// push the new connection into the pool
func (p *Pool) Push(c net.Conn) *ConnNode {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	// Add to epoll events
	f, _ := c.(*net.TCPConn).File()
	fd := int32(f.Fd())
	new := &ConnNode{
		Conn: c,
		fd:   fd,
	}
	p.eventAdd(fd)
	if p.head == nil && p.tail == nil {
		p.head = new
		p.tail = new
	} else {
		new.After(p.tail)
		p.tail = new
	}
	p.len++
	return new
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

// dialOne creates a TCP Connection,
func (p *Pool) dialWith(ctx context.Context) (net.Conn, error) {
	return p.dialer.DialContext(ctx, "tcp", p.remote)
}

func (p *Pool) epollInit() error {
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		return err
	}
	p.epoll.fd = epfd
	return nil
}

// epoll event add
func (p *Pool) eventAdd(fd int32) error {
	var event syscall.EpollEvent
	event.Events = syscall.EPOLLIN | EPOLLET
	event.Fd = fd
	if err := syscall.EpollCtl(p.epoll.fd, syscall.EPOLL_CTL_ADD, int(fd), &event); err != nil {
		return err
	}
	return nil
}

// epoll event add
func (p *Pool) eventDel(fd int32) error {
	if err := syscall.EpollCtl(p.epoll.fd, syscall.EPOLL_CTL_DEL, int(fd), nil); err != nil {
		return err
	}
	return nil
}

// epoll daemon
func (p *Pool) epollRun() {
	for {
		n, err := syscall.EpollWait(p.epoll.fd, p.epoll.events[:], -1)
		if err != nil {
			select {
			case <-p.isClose.Done():
				return
			default:
				log.Println(err)
				continue
			}
		}
		p.markReadable(n)
	}
}

func (p *Pool) EpollClose() {
	p.close()
	syscall.Close(p.epoll.fd)
}

// initialize the connection first.
func (p *Pool) connInit(minSize int32) {
	for i := int32(0); i < minSize; i++ {
		c, err := p.dialOne()
		if err != nil {
			continue
		}
		p.Push(c)
	}
}

func (p *Pool) Remote() string {
	return p.remote
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
	p := &Pool{remote: remote, connContext: c, dialer: d, readableQueue: make(chan net.Conn, MIN_READABLE_QUEUE_SIZE), maxSize: GetSysMax()}
	p.reconnect = ATTEMPT_RECONNECT
	p.reconnectTimeout = 5 * time.Minute
	p.isClose, p.close = context.WithCancel(context.Background())
	if err := p.epollInit(); err != nil {
		return nil, err
	}
	p.connInit(m)
	go p.epollRun()
	return p, nil
}
