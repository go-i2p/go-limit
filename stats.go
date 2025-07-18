package limitedlistener

import "net"

// Stats provides current listener statistics
type Stats struct {
	ActiveConnections int64
	MaxConnections    int
	RateLimit         float64
}

// GetStats returns current listener statistics
func (l *LimitedListener) GetStats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()

	var rateLimit float64
	if l.limiter != nil {
		rateLimit = float64(l.limiter.Limit())
	}

	return Stats{
		ActiveConnections: l.activeConns,
		MaxConnections:    l.maxConns,
		RateLimit:         rateLimit,
	}
}

// Close implements graceful shutdown
func (l *LimitedListener) Close() error {
	l.mu.Lock()
	// Create a slice of connections to close, so we can release the lock
	connections := make([]net.Conn, 0, len(l.activeSet))
	for conn := range l.activeSet {
		connections = append(connections, conn)
	}
	l.mu.Unlock()

	// Close all active connections outside the lock to avoid deadlock
	for _, conn := range connections {
		conn.Close()
	}

	return l.Listener.Close()
}
