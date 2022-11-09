package pool

import (
	"net"
	"sync"
	"sync/atomic"
)

const (
	UP int32 = iota
	DOWN
)

type ConnNode struct {
	Conn     net.Conn
	Lock     sync.Mutex
	prev     *ConnNode
	next     *ConnNode
	fd       int32
	isBad    bool
	waitNum  int32
	isClosed int32
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
	cn.prev = n
	cn.prev.next = cn
}

// move the node ahead n
func (cn *ConnNode) Before(n *ConnNode) {
	cn.next = n
	cn.next.prev = cn
}

// the connection is busy or not
func (cn *ConnNode) IsBusy() bool {
	return atomic.LoadInt32(&cn.waitNum) > 1
}

// wait until the connection is available
func (cn *ConnNode) Wait() {
	atomic.AddInt32(&cn.waitNum, 1)
	defer atomic.AddInt32(&cn.waitNum, -1)
	cn.Lock.Lock()
}

// connection is available or not
func (cn *ConnNode) IsAvailable() bool {
	return cn.Lock.TryLock()
}

// set the connection down
func (cn *ConnNode) Down() bool {
	if atomic.CompareAndSwapInt32(&cn.isClosed, UP, DOWN) {
		cn.isBad = true
		return true
	}
	return false
}

// set the connection up
func (cn *ConnNode) Up() bool {
	if atomic.CompareAndSwapInt32(&cn.isClosed, DOWN, UP) {
		cn.isBad = false
		return true
	}
	return false
}

// the connection is closed or not
func (cn *ConnNode) IsClosed() bool {
	return cn.isClosed == DOWN
}

// the connection is down or not
func (cn *ConnNode) IsDown() bool {
	return cn.isBad
}
