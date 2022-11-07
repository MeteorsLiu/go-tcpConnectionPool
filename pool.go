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
	MIN_SIZE                = 8
	EPOLL_MAX_SIZE          = 1024
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

type Pool struct {
	head             *ConnNode
	tail             *ConnNode
	mutex            sync.RWMutex
	len              int32
	remote           string
	maxSize          int32
	reconnect        int
	reconnectTimeout time.Duration
	connContext      context.Context
	dialer           *net.Dialer
	isClose          context.Context
	close            context.CancelFunc
	readableQueue    chan net.Conn
	readerBufferCh   chan *[]byte
	bufferPool       sync.Pool
	epoll            struct {
		events []syscall.EpollEvent
		fd     int
		len    int64
	}
}

// dialOne creates a TCP Connection,
func (p *Pool) dialOne() (net.Conn, error) {
	var c net.Conn
	var err error
	// wait until write lock unlocks
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.connContext == nil {
		c, err = p.dialer.Dial("tcp", p.remote)
	} else {
		c, err = p.dialer.DialContext(p.connContext, "tcp", p.remote)
	}
	return c, err

}

// dialOne creates a TCP Connection,
func (p *Pool) dialWith(ctx context.Context) (net.Conn, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.dialer.DialContext(ctx, "tcp", p.remote)
}

func (p *Pool) epollInit() error {
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		return err
	}
	p.epoll.fd = epfd
	p.epoll.events = make([]syscall.EpollEvent, EPOLL_MAX_SIZE)
	return nil
}

// epoll event add
func (p *Pool) eventAdd(fd int32) error {
	var event syscall.EpollEvent
	event.Events = syscall.EPOLLIN | syscall.EPOLLRDHUP | EPOLLET
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

// move the node to the head
func (p *Pool) MoveToHead(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.tail == cp {
		p.tail = cp.prev
	}
	cp.Before(p.head)
	p.head = cp
}

// move the node to the tail
func (p *Pool) MoveToTail(cp *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.head == cp {
		p.head = cp.next
	}
	cp.After(p.tail)
	p.tail = cp
}

func (p *Pool) Remote() string {
	return p.remote
}

func (p *Pool) Remove(n *ConnNode) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if n.prev != nil {
		n.prev.next = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	}
	if p.head == n {
		p.head = n.next
	}
	if p.tail == n {
		p.tail = n.prev
	}
	// make sure it will not unlock if the lock doesn't lock on.
	n.Lock.TryLock()
	n.Lock.Unlock()
	n = nil
	p.len--
}

// this is NOT A THREAD-SAFE method
// please check the Reconnect/Push function for the correct usage.
// before calling this, you MUST lock the mutex lock of ConnNode.
// otherwise, it will cause the data race problem.
// if this connection is in used, removing this will cause nil pointer panic.
func (p *Pool) RemoveConn(cn *ConnNode) {
	p.eventDel(cn.fd)
	_ = atomic.AddInt64(&p.epoll.len, -1)
	cn.Conn.Close()
}

// this is NOT A THREAD-SAFE method
// please check the Reconnect/Push function for the correct usage.
func (p *Pool) AddConn(cn *ConnNode) {
	f, _ := cn.Conn.(*net.TCPConn).File()
	fd := int32(f.Fd())
	cn.fd = fd
	p.eventAdd(fd)
	_ = atomic.AddInt64(&p.epoll.len, 1)
}

func (p *Pool) Reconnect(cn *ConnNode) {
	if cn.IsDown() || cn.IsClosed() {
		return
	}
	// shut it down first
	// this is for ReadFrom method, because that method will not be returned immediately if the connection is bad.
	if cn.Conn != nil {
		// the connection has been closed.
		if !cn.Down() {
			return
		}
		p.RemoveConn(cn)
	}
	cn.Lock.Lock()
	defer func() {
		if cn != nil {
			cn.Lock.Unlock()
		}
	}()
	// we need to be careful to handle the closed connection.
	// grab the lock first
	cn.Conn = nil
	timeout, cancel := context.WithTimeout(context.Background(), p.reconnectTimeout)
	defer cancel()
	var err error

	wait := 500 * time.Millisecond
	for i := 0; i < p.reconnect; i++ {
		cn.Conn, err = p.dialWith(timeout)
		if err == nil && cn.Conn != nil {
			p.AddConn(cn)
			cn.Up()
			return
		}
		time.Sleep(wait * time.Duration(i+1))
	}
	p.Remove(cn)
	cn = nil
}

// this is for creating a new connection.
// push the new connection into the pool
func (p *Pool) Push(c net.Conn) *ConnNode {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	new := &ConnNode{
		Conn: c,
	}
	p.AddConn(new)
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

// set dial new remote
func (p *Pool) SetRemote(remote string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.remote = remote
}

// set new dialer
func (p *Pool) SetDialer(d *net.Dialer) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.dialer = d
}

// set new dial context
func (p *Pool) SetDialContext(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.connContext = ctx
}

// set new max conn
func (p *Pool) SetMaxConn(max int32) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.maxSize = max
}

// set new min conn
func (p *Pool) SetMinConn(min int32) error {
	p.mutex.RLock()
	len := p.len
	max := p.maxSize
	p.mutex.RUnlock()
	for len < min && min < max {
		c, err := p.dialOne()
		if err != nil {
			return err
		}
		p.Push(c)
		len++
	}
	return nil
}

func (p *Pool) ReplaceRemote(remote string) error {
	p.mutex.RLock()
	node := p.head
	p.mutex.RUnlock()
	p.SetRemote(remote)
	for node != nil {
		c, err := p.dialOne()
		if err != nil {
			return err
		}
		// grab the lock first
		node.Lock.Lock()
		p.RemoveConn(node)
		node.Conn = c
		p.AddConn(node)
		node.Lock.Unlock()

		p.mutex.RLock()
		node = node.next
		p.mutex.RUnlock()
	}
	return nil
}

// Deprecated
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
	var node *ConnNode
	succ := false
	p.mutex.RLock()
	node = p.head
	p.mutex.RUnlock()
	for !succ && node != nil {
		// skip bad connections
		if node.IsDown() || node.IsClosed() {
			p.mutex.RLock()
			node = node.next
			p.mutex.RUnlock()
			continue
		}
		// try to grab the lock.
		// trylock will not pause the goroutine.

		succ = node.IsAvailable()

		if !succ {
			p.mutex.RLock()
			node = node.next
			p.mutex.RUnlock()
		}
	}

	if node == nil {
		// tries are all fail
		// stage into the lock strvation
		if atomic.LoadInt32(&p.len) < p.maxSize {
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
			if node == nil {
				return nil, NO_AVAILABLE_CONN
			}
			if node.IsDown() || node.IsClosed() {
				return nil, NO_AVAILABLE_CONN
			}
			node.Lock.Lock()
		}

	}

	return node, nil
}

// put the writable connection into the pool.
func (p *Pool) Put(c *ConnNode) {
	c.Lock.Unlock()
	if p.head != c {
		p.MoveToHead(c)
	}
}

// close all connection.
func (p *Pool) Close() {
	p.EpollClose()
	node := p.head
	for node != nil {
		// close the connection
		// if there is someone reading or writing, it will return EOF immediately.
		if node.Conn != nil {
			node.Conn.Close()
		}
		node = node.next
	}
}

func (p *Pool) Read(b []byte) (n int, err error) {
	buf := <-p.readerBufferCh
	defer p.bufferPool.Put(buf)
	n = copy(b, *buf)
	return
}

func (p *Pool) readWorker() {
	c := <-p.readableQueue
	b := *p.bufferPool.Get().(*[]byte)
	n, err := c.Read(b[0:cap(b)])
	if err != nil {
		// TODO
		log.Println(err)
	}
	b = b[0:n]
	select {
	case p.readerBufferCh <- &b:
	default:
		// don't block
	}

}

// add readable connections to the readableQueue
func (p *Pool) markReadable(n int) {
	p.mutex.RLock()
	node := p.head
	p.mutex.RUnlock()
	hasBad := false
	for node != nil {
		for i := 0; i < n; i++ {
			if p.epoll.events[i].Fd == node.fd {
				if p.epoll.events[i].Events&(syscall.EPOLLERR|syscall.EPOLLRDHUP|syscall.EPOLLHUP) != 0 {
					hasBad = true
					go p.Reconnect(node)
				} else if p.epoll.events[i].Events&syscall.EPOLLIN != 0 {
					p.readableQueue <- node.Conn
					go p.readWorker()
				}

			}
		}
		p.mutex.RLock()
		node = node.next
		p.mutex.RUnlock()
	}
	if hasBad {
		log.Println("Some connections are disconnected. Try to reconnect...")
	}
}

// epoll daemon
func (p *Pool) epollRun() {
	for {
		size := atomic.LoadInt64(&p.epoll.len)
		if size == 0 {
			select {
			case <-p.isClose.Done():
				return
			default:
				time.Sleep(time.Second)
				continue
			}
		}
		if size > EPOLL_MAX_SIZE {
			// resize if the number of connection is more than 1024
			p.epoll.events = make([]syscall.EpollEvent, size)
		}
		n, err := syscall.EpollWait(p.epoll.fd, p.epoll.events[:size], -1)
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
	p := &Pool{remote: remote, connContext: c, dialer: d, maxSize: GetSysMax()}
	p.bufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 4096)
			return &b
		},
	}
	p.readableQueue = make(chan net.Conn, MIN_READABLE_QUEUE_SIZE)
	p.readerBufferCh = make(chan *[]byte, MIN_READABLE_QUEUE_SIZE)
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
