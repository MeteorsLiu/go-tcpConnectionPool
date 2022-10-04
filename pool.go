package pool

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	MIN_SIZE = 128
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
	head          *ConnNode
	tail          *ConnNode
	mutex         sync.RWMutex
	num           int32
	remote        string
	maxSize       int
	minConsumer   int32
	connContext   context.Context
	dialer        *net.Dialer
	isClose       context.Context
	close         context.CancelFunc
	readableQueue chan net.Conn
	epoll         struct {
		events [MIN_SIZE]syscall.EpollEvent
		fd     uintptr
	}
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
func (p *Pool) MarkReadable(n int) {
	p.mutex.RLock()
	node := p.head
	p.mutex.RUnlock()
	for node != nil {
		for i := 0; i < n; i++ {
			if p.epoll.events[i].Fd == node.fd {
				select {
				case p.readableQueue <- node.Conn:
				default:
				}
			}
		}
		p.mutex.RLock()
		node = node.next
		p.mutex.RUnlock()
	}
}

func (p *Pool) GetReadableConn() (net.Conn, error) {
	var c net.Conn
	select {
	case c = <-p.readableQueue:
		return c, nil
	case <-p.isClose.Done():
		return nil, POOL_CLOSED
	}

}
func (p *Pool) Get() (*ConnNode, error) {
	p.mutex.RLock()
	node := p.head
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
	gminC := atomic.LoadInt32(&p.minConsumer)
	if gminC > minC || gminC == 0 {
		_ = atomic.SwapInt32(&p.minConsumer, minC)
	}
	return node, nil
}
func (p *Pool) Put(c *ConnNode) {
	consumer := atomic.AddInt32(&c.consumer, -1)
	defer c.Lock.Unlock()
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
	p.close()
	syscall.Close(syscall.Handle(p.epoll.fd))
	node := p.head
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

func (p *Pool) epollInit() error {
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		return err
	}
	p.epoll.fd = epfd
	return nil
}

func (p *Pool) eventAdd(fd int32) error {
	var event syscall.EpollEvent
	event.Events = syscall.EPOLLIN
	event.Fd = fd
	if err := syscall.EpollCtl(p.epoll.fd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}
	return nil
}

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
		p.MarkReadable(n)
	}
}
func (p *Pool) connInit(minSize int32) error {
	errCh := make(chan error, 1)
	done, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	for i := int32(0); i < minSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := p.dialOne()
			if err != nil {
				select {
				case errCh <- err:
					return
				default:
				}
			}
			select {
			case <-done.Done():
				return
			default:
			}
			p.Push(c)
		}()
	}
	select {
	case err := <-errCh:
		cancel()
		return err
	default:
		wg.Wait()
	}
	return nil
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
	p := &Pool{remote: remote, connContext: c, dialer: d, readableQueue: make(chan net.Conn, MIN_SIZE)}

	p.isClose, p.close = context.WithCancel(context.Background())
	if err := p.epollInit(); err != nil {
		return nil, err
	}

	if err := p.connInit(m); err != nil {
		return nil, err
	}
	go p.epollRun()
	return p, nil
}
