package limitedlistener

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
	defer l.mu.Unlock()

	// Close all active connections
	for conn := range l.activeSet {
		conn.Close()
	}

	close(l.done)
	return l.Listener.Close()
}
