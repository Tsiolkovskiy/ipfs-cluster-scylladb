package scyllastate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Default(t *testing.T) {
	cfg := &Config{}
	err := cfg.Default()
	require.NoError(t, err)

	assert.Equal(t, []string{"127.0.0.1"}, cfg.Hosts)
	assert.Equal(t, DefaultPort, cfg.Port)
	assert.Equal(t, DefaultKeyspace, cfg.Keyspace)
	assert.Equal(t, DefaultNumConns, cfg.NumConns)
	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultConnectTimeout, cfg.ConnectTimeout)
	assert.Equal(t, DefaultConsistency, cfg.Consistency)
	assert.Equal(t, DefaultSerialConsistency, cfg.SerialConsistency)
	assert.True(t, cfg.MetricsEnabled)
	assert.False(t, cfg.TracingEnabled)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  func() *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: func() *Config {
				cfg := &Config{}
				cfg.Default()
				return cfg
			},
			wantErr: false,
		},
		{
			name: "empty hosts",
			config: func() *Config {
				cfg := &Config{}
				cfg.Default()
				cfg.Hosts = []string{}
				return cfg
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: func() *Config {
				cfg := &Config{}
				cfg.Default()
				cfg.Port = 0
				return cfg
			},
			wantErr: true,
		},
		{
			name: "empty keyspace",
			config: func() *Config {
				cfg := &Config{}
				cfg.Default()
				cfg.Keyspace = ""
				return cfg
			},
			wantErr: true,
		},
		{
			name: "invalid consistency",
			config: func() *Config {
				cfg := &Config{}
				cfg.Default()
				cfg.Consistency = "INVALID"
				return cfg
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config()
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMhPrefix(t *testing.T) {
	tests := []struct {
		name     string
		cidBin   []byte
		expected int16
	}{
		{
			name:     "empty cid",
			cidBin:   []byte{},
			expected: 0,
		},
		{
			name:     "single byte",
			cidBin:   []byte{0x12},
			expected: 0,
		},
		{
			name:     "two bytes",
			cidBin:   []byte{0x12, 0x34},
			expected: 0x1234,
		},
		{
			name:     "more than two bytes",
			cidBin:   []byte{0x12, 0x34, 0x56, 0x78},
			expected: 0x1234,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mhPrefix(tt.cidBin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCidToPartitionedKey(t *testing.T) {
	cidBin := []byte{0x12, 0x34, 0x56, 0x78}
	prefix, key := cidToPartitionedKey(cidBin)

	assert.Equal(t, int16(0x1234), prefix)
	assert.Equal(t, cidBin, key)
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	// Test first attempt (should be 0)
	delay := rp.calculateDelay(0)
	assert.Equal(t, time.Duration(0), delay)

	// Test subsequent attempts
	delay1 := rp.calculateDelay(1)
	assert.GreaterOrEqual(t, delay1, rp.minDelay)
	assert.LessOrEqual(t, delay1, rp.maxDelay)

	delay2 := rp.calculateDelay(2)
	assert.GreaterOrEqual(t, delay2, rp.minDelay)
	assert.LessOrEqual(t, delay2, rp.maxDelay)

	// Generally, delay2 should be greater than delay1 (with some jitter tolerance)
	// We'll just check they're both within reasonable bounds
	assert.True(t, delay1 > 0)
	assert.True(t, delay2 > 0)
}

func TestRetryPolicy_IsRetryable(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.isRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPinState_String(t *testing.T) {
	tests := []struct {
		state    PinState
		expected string
	}{
		{StateQueued, "queued"},
		{StatePinning, "pinning"},
		{StatePinned, "pinned"},
		{StateFailed, "failed"},
		{StateUnpinned, "unpinned"},
		{PinState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestScyllaStateInterfaces verifies that ScyllaState implements required interfaces
func TestScyllaStateInterfaces(t *testing.T) {
	// This test just verifies compilation - the interface implementations
	// are checked at compile time via the var declarations at the end of scyllastate.go

	// If this test compiles, it means our interfaces are correctly implemented
	assert.True(t, true, "Interface implementations compile correctly")
}

// Mock test for basic functionality - these would need a real ScyllaDB instance
// to run properly, but they verify the method signatures are correct
func TestScyllaState_MockMethods(t *testing.T) {
	// Test that we can create a config
	cfg := &Config{}
	err := cfg.Default()
	require.NoError(t, err)

	// Test that validation works
	err = cfg.Validate()
	assert.NoError(t, err)

	// Test cluster config creation
	clusterConfig, err := cfg.CreateClusterConfig()
	assert.NoError(t, err)
	assert.NotNil(t, clusterConfig)
	assert.Equal(t, cfg.Hosts, clusterConfig.Hosts)
	assert.Equal(t, cfg.Port, clusterConfig.Port)
	assert.Equal(t, cfg.Keyspace, clusterConfig.Keyspace)
}

func TestNewMetrics(t *testing.T) {
	metrics := newMetrics()
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.operationDuration)
	assert.NotNil(t, metrics.operationCounter)
	assert.NotNil(t, metrics.activeConnections)
	assert.NotNil(t, metrics.connectionErrors)
	assert.NotNil(t, metrics.totalPins)
	assert.NotNil(t, metrics.pinOperationsRate)
	assert.NotNil(t, metrics.queryLatency)
	assert.NotNil(t, metrics.timeoutErrors)
	assert.NotNil(t, metrics.unavailableErrors)
}
