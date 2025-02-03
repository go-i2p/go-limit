package limitedlistener

type LimitedConfig struct {
	MaxConns  int
	RateLimit float64
}
