package pool

import (
	"net"
	"sync"
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

func (cn *ConnNode) IsAvailable() bool {
	return cn.Lock.TryLock()
}
