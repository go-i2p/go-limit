package limitedlistener

import (
	"fmt"
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
		connChan: make(chan net.Conn, 100),
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

func TestActiveSetDataRace(t *testing.T) {
	mockL := newMockListener()
	defer mockL.Close()

	limited := NewLimitedListener(mockL, WithMaxConnections(10))

	// Pre-populate with exactly enough connections to avoid blocking
	for i := 0; i < 15; i++ {
		mockL.sendConn(&mockConn{})
	}

	var wg sync.WaitGroup
	connections := make([]net.Conn, 0, 10)
	connMutex := sync.Mutex{}

	// Accept 10 connections concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := limited.Accept()
			if err == nil && conn != nil {
				connMutex.Lock()
				connections = append(connections, conn)
				connMutex.Unlock()
			}
		}()
	}

	// Wait for accepts to complete
	wg.Wait()

	// Now close some connections concurrently to test the race condition
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			connMutex.Lock()
			if len(connections) > 0 {
				conn := connections[len(connections)-1]
				connections = connections[:len(connections)-1]
				connMutex.Unlock()
				conn.Close()
			} else {
				connMutex.Unlock()
			}
		}()
	}

	// Also test listener close concurrent with connection operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // Brief delay to let some closes happen
		limited.Close()
	}()

	// Wait for all operations to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no race detected
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out - possible deadlock or race")
	}

	// Clean up any remaining connections
	connMutex.Lock()
	for _, conn := range connections {
		conn.Close()
	}
	connMutex.Unlock()
}

func TestSlotReservationRaceCondition(t *testing.T) {
	mockL := newMockListener()
	defer mockL.Close()

	// Create a listener with a very small limit
	limited := NewLimitedListener(mockL, WithMaxConnections(3))

	// Pre-populate with enough connections to stress test the limit
	for i := 0; i < 20; i++ {
		mockL.sendConn(&mockConn{})
	}

	var wg sync.WaitGroup
	var successfulConnections []net.Conn
	var connMutex sync.Mutex

	// Start many goroutines trying to accept connections concurrently
	numGoroutines := 15
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := limited.Accept()
			if err == nil && conn != nil {
				connMutex.Lock()
				successfulConnections = append(successfulConnections, conn)
				connMutex.Unlock()
			}
		}()
	}

	// Wait for all accepts to complete
	wg.Wait()

	// Verify that we didn't exceed the connection limit
	connMutex.Lock()
	actualCount := len(successfulConnections)
	connMutex.Unlock()

	// Check that activeConns matches the actual number of successful connections
	stats := limited.GetStats()

	if actualCount != int(stats.ActiveConnections) {
		t.Errorf("Slot reservation race detected: actualCount=%d, activeConns=%d",
			actualCount, stats.ActiveConnections)
	}

	if actualCount > 3 {
		t.Errorf("Connection limit exceeded: expected max 3, got %d successful connections", actualCount)
	}

	// Clean up
	connMutex.Lock()
	for _, conn := range successfulConnections {
		conn.Close()
	}
	connMutex.Unlock()

	t.Logf("Successfully limited to %d connections (limit: 3)", actualCount)
}

// doubleCloseMockConn tracks how many times Close() is called
type doubleCloseMockConn struct {
	*mockConn
	closeCount int32
	mu         sync.Mutex
}

func (d *doubleCloseMockConn) Close() error {
	d.mu.Lock()
	d.closeCount++
	closedTimes := d.closeCount
	d.mu.Unlock()

	if closedTimes > 1 {
		panic(fmt.Sprintf("Connection closed %d times - double close detected!", closedTimes))
	}

	return d.mockConn.Close()
}

func (d *doubleCloseMockConn) getCloseCount() int32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closeCount
}

func TestDoubleClosePreevention(t *testing.T) {
	mockL := newMockListener()
	defer mockL.Close()

	limited := NewLimitedListener(mockL, WithMaxConnections(5))

	// Create special mock connections that track close calls
	doubleCloseMocks := make([]*doubleCloseMockConn, 3)
	for i := 0; i < 3; i++ {
		doubleCloseMocks[i] = &doubleCloseMockConn{
			mockConn: &mockConn{},
		}
		mockL.sendConn(doubleCloseMocks[i])
	}

	// Accept the connections
	var connections []net.Conn
	for i := 0; i < 3; i++ {
		conn, err := limited.Accept()
		if err != nil {
			t.Fatalf("Failed to accept connection %d: %v", i, err)
		}
		connections = append(connections, conn)
	}

	// Close the first connection manually (user close)
	err := connections[0].Close()
	if err != nil {
		t.Fatalf("Failed to close connection manually: %v", err)
	}

	// Verify it was closed once
	if doubleCloseMocks[0].getCloseCount() != 1 {
		t.Errorf("Expected connection 0 to be closed once, got %d times", doubleCloseMocks[0].getCloseCount())
	}

	// Now close the listener - this should close remaining connections but NOT double-close the first one
	err = limited.Close()
	if err != nil {
		t.Fatalf("Failed to close listener: %v", err)
	}

	// Verify close counts
	for i, mock := range doubleCloseMocks {
		closeCount := mock.getCloseCount()
		if closeCount != 1 {
			t.Errorf("Connection %d should be closed exactly once, but was closed %d times", i, closeCount)
		}
	}

	// Test idempotent close - closing already closed connections should be safe
	for i, conn := range connections {
		err := conn.Close()
		if err != nil {
			t.Errorf("Idempotent close of connection %d should not return error: %v", i, err)
		}

		// Close count should still be 1 (idempotent)
		if doubleCloseMocks[i].getCloseCount() != 1 {
			t.Errorf("After idempotent close, connection %d close count should still be 1, got %d", i, doubleCloseMocks[i].getCloseCount())
		}
	}

	t.Log("Double close prevention test passed - all connections closed exactly once")
}
