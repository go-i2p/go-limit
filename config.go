package limitedlistener

type LimitConfig struct {
	MaxConns  int
	RateLimit float64
}
