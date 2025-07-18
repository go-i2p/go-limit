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
	closed   bool
	mu       sync.Mutex
}

// Close implements net.Conn Close with connection tracking
func (c *trackedConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil // Already closed, return without error
	}
	c.closed = true
	c.mu.Unlock()

	var closeErr error
	c.once.Do(func() {
		// Remove from active set and decrement counter atomically
		c.listener.mu.Lock()
		delete(c.listener.activeSet, c)
		c.listener.activeConns--
		c.listener.mu.Unlock()

		// Close the underlying connection
		closeErr = c.Conn.Close()
	})
	return closeErr
}
