package limitedlistener

import "net"

// NewLimitedListener creates a new connection-limited listener
func NewLimitedListener(listener net.Listener, opts ...Option) *LimitedListener {
	l := &LimitedListener{
		Listener:  listener,
		maxConns:  1000, // default limit
		activeSet: make(map[net.Conn]struct{}),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// Accept accepts a connection with limiting logic
func (l *LimitedListener) Accept() (net.Conn, error) {
	// Check if rate limit is exceeded
	if l.limiter != nil {
		if !l.limiter.Allow() {
			return nil, ErrRateLimitExceeded
		}
	}

	// Check concurrent connection limit
	l.mu.Lock()
	if l.maxConns > 0 && l.activeConns >= int64(l.maxConns) {
		l.mu.Unlock()
		return nil, ErrMaxConnsReached
	}
	l.mu.Unlock()

	// Accept the connection first (without reserving a slot yet)
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Now that we have a successful connection, atomically check limits and add tracking
	l.mu.Lock()
	// Double-check the limit after accepting (another goroutine might have accepted while we were blocked)
	if l.maxConns > 0 && l.activeConns >= int64(l.maxConns) {
		l.mu.Unlock()
		// Close the connection we just accepted since we're over the limit
		conn.Close()
		return nil, ErrMaxConnsReached
	}

	// Wrap the connection for tracking
	tracked := &trackedConn{
		Conn:     conn,
		listener: l,
	}

	// Atomically add to both counter and active set
	l.activeConns++
	l.activeSet[tracked] = struct{}{}
	l.mu.Unlock()

	return tracked, nil

}
