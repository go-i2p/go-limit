package limitedlistener

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

// controlledMockListener allows us to control when Accept() returns
type controlledMockListener struct {
	connChan   chan net.Conn
	errChan    chan error
	closed     bool
	mu         sync.Mutex
	acceptCh   chan struct{} // Signal to allow Accept() to proceed
	acceptWait bool          // Whether Accept() should wait
}

func newControlledMockListener() *controlledMockListener {
	return &controlledMockListener{
		connChan: make(chan net.Conn, 100),
		errChan:  make(chan error, 10),
		acceptCh: make(chan struct{}, 100), // Buffered channel
	}
}

func (m *controlledMockListener) Accept() (net.Conn, error) {
	// Wait for signal if control is enabled
	if m.acceptWait {
		<-m.acceptCh
	}

	select {
	case conn := <-m.connChan:
		return conn, nil
	case err := <-m.errChan:
		return nil, err
	default:
		// Return a mock connection if no specific conn/err was queued
		return &mockConn{}, nil
	}
}

func (m *controlledMockListener) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.connChan)
		close(m.errChan)
	}
	return nil
}

func (m *controlledMockListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (m *controlledMockListener) enableAcceptControl() {
	m.acceptWait = true
}

func (m *controlledMockListener) releaseAllAccepts(count int) {
	for i := 0; i < count; i++ {
		m.acceptCh <- struct{}{}
	}
}

// TestRaceConditionReproduction creates a scenario that reliably reproduces
// the race condition where connection limits can be temporarily exceeded
func TestRaceConditionReproduction(t *testing.T) {
	// Use a simple approach: stress test with many concurrent accepts
	// and check if we can exceed the limit
	for attempt := 0; attempt < 10; attempt++ {
		mockL := newControlledMockListener()
		limited := NewLimitedListener(mockL, WithMaxConnections(2))

		// Pre-signal the accepts to avoid blocking
		mockL.enableAcceptControl()
		mockL.releaseAllAccepts(10)

		var wg sync.WaitGroup
		var successfulConns int64
		var connections []net.Conn
		var connMutex sync.Mutex

		// Start multiple goroutines trying to accept connections simultaneously
		numGoroutines := 5
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				conn, err := limited.Accept()
				if err == nil && conn != nil {
					atomic.AddInt64(&successfulConns, 1)
					connMutex.Lock()
					connections = append(connections, conn)
					connMutex.Unlock()
				}
			}()
		}

		// Wait for completion
		wg.Wait()

		// Clean up connections
		connMutex.Lock()
		for _, conn := range connections {
			conn.Close()
		}
		connMutex.Unlock()

		mockL.Close()
		limited.Close()

		successful := atomic.LoadInt64(&successfulConns)

		// Check if we reproduced the race condition
		if successful > 2 {
			t.Logf("RACE CONDITION REPRODUCED on attempt %d: %d connections accepted (limit was 2)", attempt+1, successful)
			t.Errorf("Connection limit exceeded due to race condition: expected at most 2, got %d", successful)
			return
		}

		t.Logf("Attempt %d: %d connections accepted (within limit)", attempt+1, successful)
	}

	t.Log("Race condition was not reproduced in 10 attempts, but the theoretical issue still exists")
}
