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

	// Check concurrent connection limit and reserve a slot atomically
	l.mu.Lock()
	if l.maxConns > 0 && l.activeConns >= int64(l.maxConns) {
		l.mu.Unlock()
		return nil, ErrMaxConnsReached
	}

	// Reserve the connection slot before accepting
	l.activeConns++
	l.mu.Unlock()

	// Accept the connection
	conn, err := l.Listener.Accept()
	if err != nil {
		// If accept fails, we need to release the reserved slot
		l.mu.Lock()
		l.activeConns--
		l.mu.Unlock()
		return nil, err
	}

	// Wrap the connection for tracking
	tracked := &trackedConn{
		Conn:     conn,
		listener: l,
	}

	// Add to active set (we already incremented the counter)
	l.mu.Lock()
	l.activeSet[tracked] = struct{}{}
	l.mu.Unlock()

	return tracked, nil

}
