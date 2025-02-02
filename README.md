# go-limit

A simple, thread-safe connection limiter for Go's `net.Listener` that manages concurrent connections and rate limiting.
Factored out from sam-forwarder to be used in go-i2ptunnel.

## Install

```bash
go get github.com/go-i2p/go-limit
```

## Quick Start

```go
package main

import (
    "log"
    "net"
    "github.com/go-i2p/go-limit"
)

func main() {
    base, _ := net.Listen("tcp", ":8080")
    
    listener := limitedlistener.NewLimitedListener(base,
        limitedlistener.WithMaxConnections(1000),  // max concurrent
        limitedlistener.WithRateLimit(100))        // per second
    defer listener.Close()

    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Printf("Accept error: %v", err)
            continue
        }
        go handleConnection(conn)
    }
}
```

## Features

- Limit concurrent connections
- Rate limiting (connections per second)
- Connection tracking
- Real-time statistics

## Configuration

```go
// Set maximum concurrent connections
WithMaxConnections(1000)

// Set rate limit (connections per second)
WithRateLimit(100.0)

// Get current stats
stats := listener.GetStats()
```

## License

MIT License

## Support

Issues and PRs welcome at [github.com/go-i2p/go-limit](https://github.com/go-i2p/go-limit)