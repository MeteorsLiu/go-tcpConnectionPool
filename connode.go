package pool

import (
	"net"
	"sync"
	"sync/atomic"
)

type ConnNode struct {
	Conn     net.Conn
	Lock     sync.Mutex
	prev     *ConnNode
	next     *ConnNode
	fd       int32
	isBad    bool
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
	if cn.prev != nil {
		cn.prev.next = cn.next
	}
	if cn.next != nil {
		cn.next.prev = cn.prev
	}
	cn.prev = n
	cn.next = n.next
	cn.prev.next = cn
	if cn.next != nil {
		cn.next.prev = cn
	}
}

// move the node ahead n
func (cn *ConnNode) Before(n *ConnNode) {
	if cn.prev != nil {
		cn.prev.next = cn.next
	}
	if cn.next != nil {
		cn.next.prev = cn.prev
	}
	cn.prev = n.prev
	cn.next = n
	cn.next.prev = cn
	if cn.prev != nil {
		cn.prev.next = cn
	}
}

func (cn *ConnNode) IsAvailable() bool {
	return cn.Lock.TryLock()
}
func (cn *ConnNode) Down() bool {
	if atomic.CompareAndSwapInt32(&cn.isClosed, 0, 1) {
		cn.isBad = true
		return true
	}
	return false
}
func (cn *ConnNode) Up() bool {
	if atomic.CompareAndSwapInt32(&cn.isClosed, 1, 0) {
		cn.isBad = false
		return true
	}
	return false
}
func (cn *ConnNode) IsClosed() bool {
	return cn.isClosed == 1
}

func (cn *ConnNode) IsDown() bool {
	return cn.isBad
}
