package limitedlistener

import (
	"net"
	"sync"
)

// trackedConn wraps a net.Conn to track its lifecycle
type trackedConn struct {
	net.Conn
	listener *LimitedListener
	once     sync.Once
}

// Close implements net.Conn Close with connection tracking
func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.listener.mu.Lock()
		delete(c.listener.activeSet, c)
		c.listener.activeConns--
		c.listener.mu.Unlock()
	})
	return c.Conn.Close()
}
