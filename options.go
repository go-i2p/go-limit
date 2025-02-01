package limitedlistener

import "golang.org/x/time/rate"

// WithMaxConnections sets the maximum number of concurrent connections
func WithMaxConnections(max int) Option {
	return func(l *LimitedListener) {
		l.maxConns = max
	}
}

// WithRateLimit sets connections per second limit
func WithRateLimit(perSecond float64) Option {
	return func(l *LimitedListener) {
		l.limiter = rate.NewLimiter(rate.Limit(perSecond), int(perSecond))
	}
}
