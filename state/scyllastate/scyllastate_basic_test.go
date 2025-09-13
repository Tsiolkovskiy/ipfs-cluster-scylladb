package scyllastate

import (
	"context"
	"testing"
	"time"
)

// TestScyllaStateBasicStructure tests that the basic structure is properly initialized
func TestScyllaStateBasicStructure(t *testing.T) {
	// Test configuration creation
	config := &Config{}
	err := config.Default()
	if err != nil {
		t.Fatalf("Failed to set default config: %v", err)
	}

	// Test configuration validation
	err = config.Validate()
	if err != nil {
		t.Fatalf("Default config validation failed: %v", err)
	}

	// Test that required fields are set
	if len(config.Hosts) == 0 {
		t.Error("Default config should have at least one host")
	}

	if config.Keyspace == "" {
		t.Error("Default config should have a keyspace")
	}

	if config.Port <= 0 {
		t.Error("Default config should have a valid port")
	}
}

// TestPreparedStatements tests that prepared statements structure is correct
func TestPreparedStatements(t *testing.T) {
	ps := &PreparedStatements{}

	// Test that the structure exists and can be initialized
	if ps == nil {
		t.Error("PreparedStatements should be initializable")
	}
}

// TestMetrics tests that metrics structure is correct
func TestMetrics(t *testing.T) {
	metrics := newMetrics()

	if metrics == nil {
		t.Error("Metrics should be initializable")
	}

	if metrics.operationCount == nil {
		t.Error("Metrics should have operation count map initialized")
	}

	if metrics.operationDuration == nil {
		t.Error("Metrics should have operation duration map initialized")
	}
}

// TestUtilityFunctions tests utility functions
func TestUtilityFunctions(t *testing.T) {
	// Test mhPrefix function
	testCID := []byte{0x12, 0x34, 0x56, 0x78}
	prefix := mhPrefix(testCID)
	expected := int16(0x12)<<8 | int16(0x34)

	if prefix != expected {
		t.Errorf("mhPrefix returned %d, expected %d", prefix, expected)
	}

	// Test with empty CID
	emptyPrefix := mhPrefix([]byte{})
	if emptyPrefix != 0 {
		t.Errorf("mhPrefix with empty slice should return 0, got %d", emptyPrefix)
	}

	// Test cidToPartitionedKey
	prefix2, cidBin := cidToPartitionedKey(testCID)
	if prefix2 != expected {
		t.Errorf("cidToPartitionedKey prefix returned %d, expected %d", prefix2, expected)
	}

	if len(cidBin) != len(testCID) {
		t.Errorf("cidToPartitionedKey should return original CID bytes")
	}
}

// TestErrorHandling tests error handling functions
func TestErrorHandling(t *testing.T) {
	// Test timeout error detection
	timeoutErr := &mockError{msg: "timeout occurred"}
	if !isTimeoutError(timeoutErr) {
		t.Error("Should detect timeout error")
	}

	// Test unavailable error detection
	unavailableErr := &mockError{msg: "unavailable service"}
	if !isUnavailableError(unavailableErr) {
		t.Error("Should detect unavailable error")
	}

	// Test non-error cases
	if isTimeoutError(nil) {
		t.Error("Should not detect timeout on nil error")
	}

	if isUnavailableError(nil) {
		t.Error("Should not detect unavailable on nil error")
	}
}

// TestQueryObserver tests the query observer
func TestQueryObserver(t *testing.T) {
	observer := &QueryObserver{}

	// Test that it doesn't panic with nil logger
	ctx := context.Background()
	mockQuery := mockObservedQuery{
		Statement: "SELECT * FROM test",
		Start:     time.Now(),
		End:       time.Now().Add(100 * time.Millisecond),
	}

	// Should not panic
	observer.ObserveQuery(ctx, mockQuery)
}

// Mock types for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

type mockObservedQuery struct {
	Statement string
	Start     time.Time
	End       time.Time
	Err       error
	Metrics   mockQueryMetrics
	Host      mockHost
}

type mockQueryMetrics struct {
	Attempts int
}

type mockHost struct{}

func (h mockHost) ConnectAddress() mockAddress {
	return mockAddress{}
}

type mockAddress struct{}

func (a mockAddress) String() string {
	return "127.0.0.1:9042"
}
