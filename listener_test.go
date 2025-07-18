package limitedlistener

import (
	"net"
	"sync"
	"testing"
	"time"
)

// mockListener implements net.Listener for testing
type mockListener struct {
	connChan chan net.Conn
	errChan  chan error
	closed   bool
	mu       sync.Mutex
}

func newMockListener() *mockListener {
	return &mockListener{
		connChan: make(chan net.Conn, 10),
		errChan:  make(chan error, 10),
	}
}

func (m *mockListener) Accept() (net.Conn, error) {
	select {
	case conn := <-m.connChan:
		return conn, nil
	case err := <-m.errChan:
		return nil, err
	}
}

func (m *mockListener) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.connChan)
		close(m.errChan)
	}
	return nil
}

func (m *mockListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (m *mockListener) sendConn(conn net.Conn) {
	m.connChan <- conn
}

// mockConn implements net.Conn for testing
type mockConn struct {
	net.Conn
	closed bool
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) Read(b []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(b []byte) (int, error)        { return len(b), nil }
func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8081} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestConnectionLimitRaceCondition(t *testing.T) {
	mockL := newMockListener()
	defer mockL.Close()

	// Create a limited listener with a small connection limit
	limited := NewLimitedListener(mockL, WithMaxConnections(2))

	// Pre-populate the mock with connections
	for i := 0; i < 10; i++ {
		mockL.sendConn(&mockConn{})
	}

	var wg sync.WaitGroup
	results := make(chan struct {
		conn net.Conn
		err  error
	}, 10)

	// Start multiple goroutines trying to accept connections concurrently
	numGoroutines := 10
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := limited.Accept()
			results <- struct {
				conn net.Conn
				err  error
			}{conn, err}
		}()
	}

	// Wait for all goroutines to complete with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
	close(results)

	// Count results
	var successful, failed int
	var connections []net.Conn

	for result := range results {
		if result.err == nil {
			successful++
			connections = append(connections, result.conn)
		} else if result.err == ErrMaxConnsReached {
			failed++
		} else {
			t.Errorf("Unexpected error: %v", result.err)
		}
	}

	// Close all successful connections
	for _, conn := range connections {
		conn.Close()
	}

	// The key test: successful connections should not exceed the limit
	if successful > 2 {
		t.Errorf("Race condition detected: Expected at most 2 successful connections, but %d were accepted", successful)
	}

	t.Logf("Successful: %d, Failed: %d", successful, failed)
}

func TestConnectionLimitEnforcement(t *testing.T) {
	mockL := newMockListener()
	defer mockL.Close()

	limited := NewLimitedListener(mockL, WithMaxConnections(1))

	// Send mock connections
	for i := 0; i < 3; i++ {
		mockL.sendConn(&mockConn{})
	}

	// Accept first connection - should succeed
	conn1, err := limited.Accept()
	if err != nil {
		t.Fatalf("First connection should be accepted: %v", err)
	}
	defer conn1.Close()

	// Try to accept second connection - should fail with limit error
	conn2, err := limited.Accept()
	if err != ErrMaxConnsReached {
		t.Errorf("Expected ErrMaxConnsReached, got: %v", err)
	}
	if conn2 != nil {
		t.Error("Connection should be nil when limit is reached")
		conn2.Close()
	}
}
