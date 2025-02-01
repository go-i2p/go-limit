package limitedlistener

import "net"

// NewLimitedListener creates a new connection-limited listener
func NewLimitedListener(listener net.Listener, opts ...Option) *LimitedListener {
	l := &LimitedListener{
		Listener:  listener,
		maxConns:  1000, // default limit
		done:      make(chan struct{}),
		activeSet: make(map[net.Conn]struct{}),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// Accept accepts a connection with limiting logic
func (l *LimitedListener) Accept() (net.Conn, error) {
	for {
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

		// Accept the connection
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		// Wrap the connection for tracking
		tracked := &trackedConn{
			Conn:     conn,
			listener: l,
		}

		l.mu.Lock()
		l.activeConns++
		l.activeSet[tracked] = struct{}{}
		l.mu.Unlock()

		return tracked, nil
	}
}
