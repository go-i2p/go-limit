package limitedlistener

import (
	"fmt"
	"net"
	"sync"

	"golang.org/x/time/rate"
)

// LimitedListener wraps a net.Listener with connection limiting capabilities
type LimitedListener struct {
	net.Listener
	maxConns    int
	activeConns int64
	limiter     *rate.Limiter
	mu          sync.Mutex
	activeSet   map[net.Conn]struct{}
}

// Option defines the functional option pattern for configuration
type Option func(*LimitedListener)

// ErrMaxConnsReached is returned when connection limit is reached
var ErrMaxConnsReached = fmt.Errorf("maximum connections reached")

// ErrRateLimitExceeded is returned when rate limit is exceeded
var ErrRateLimitExceeded = fmt.Errorf("rate limit exceeded")
