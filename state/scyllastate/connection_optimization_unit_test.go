package scyllastate

import (
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestConfigConnectionOptimizations(t *testing.T) {
	t.Run("default_connection_pool_settings", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()

		// Verify default optimization settings
		if cfg.MaxConnectionsPerHost != DefaultMaxConnectionsPerHost {
			t.Errorf("Expected MaxConnectionsPerHost %d, got %d", DefaultMaxConnectionsPerHost, cfg.MaxConnectionsPerHost)
		}
		if cfg.MinConnectionsPerHost != DefaultMinConnectionsPerHost {
			t.Errorf("Expected MinConnectionsPerHost %d, got %d", DefaultMinConnectionsPerHost, cfg.MinConnectionsPerHost)
		}
		if cfg.MaxWaitTime != DefaultMaxWaitTime {
			t.Errorf("Expected MaxWaitTime %v, got %v", DefaultMaxWaitTime, cfg.MaxWaitTime)
		}
		if cfg.KeepAlive != DefaultKeepAlive {
			t.Errorf("Expected KeepAlive %v, got %v", DefaultKeepAlive, cfg.KeepAlive)
		}
		if cfg.PreparedStatementCacheSize != DefaultPreparedStatementCacheSize {
			t.Errorf("Expected PreparedStatementCacheSize %d, got %d", DefaultPreparedStatementCacheSize, cfg.PreparedStatementCacheSize)
		}
		if cfg.PageSize != DefaultPageSize {
			t.Errorf("Expected PageSize %d, got %d", DefaultPageSize, cfg.PageSize)
		}
		if cfg.ReconnectInterval != DefaultReconnectInterval {
			t.Errorf("Expected ReconnectInterval %v, got %v", DefaultReconnectInterval, cfg.ReconnectInterval)
		}
		if cfg.MaxReconnectAttempts != DefaultMaxReconnectAttempts {
			t.Errorf("Expected MaxReconnectAttempts %d, got %d", DefaultMaxReconnectAttempts, cfg.MaxReconnectAttempts)
		}
	})

	t.Run("token_aware_routing_configuration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()
		cfg.TokenAwareRouting = true

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Expected no validation error, got: %v", err)
		}
		if !cfg.TokenAwareRouting {
			t.Error("Expected TokenAwareRouting to be true")
		}
	})

	t.Run("dc_aware_routing_configuration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()
		cfg.DCAwareRouting = true
		cfg.LocalDC = "datacenter1"

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Expected no validation error, got: %v", err)
		}
		if !cfg.DCAwareRouting {
			t.Error("Expected DCAwareRouting to be true")
		}
		if cfg.LocalDC != "datacenter1" {
			t.Errorf("Expected LocalDC 'datacenter1', got '%s'", cfg.LocalDC)
		}
		if !cfg.IsMultiDC() {
			t.Error("Expected IsMultiDC to return true")
		}
	})

	t.Run("connection_pool_validation", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()

		// Test invalid settings
		cfg.MaxConnectionsPerHost = 0
		err := cfg.Validate()
		if err == nil {
			t.Error("Expected validation error for MaxConnectionsPerHost = 0")
		}
		if err != nil && !contains(err.Error(), "max_connections_per_host must be positive") {
			t.Errorf("Expected error about max_connections_per_host, got: %v", err)
		}

		cfg.MaxConnectionsPerHost = 10
		cfg.MinConnectionsPerHost = 15 // Greater than max
		err = cfg.Validate()
		if err == nil {
			t.Error("Expected validation error for MinConnectionsPerHost > MaxConnectionsPerHost")
		}
		if err != nil && !contains(err.Error(), "min_connections_per_host") {
			t.Errorf("Expected error about min_connections_per_host, got: %v", err)
		}

		// Test valid settings
		cfg.MinConnectionsPerHost = 5
		err = cfg.Validate()
		if err != nil {
			t.Errorf("Expected no validation error, got: %v", err)
		}
	})

	t.Run("query_optimization_validation", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()

		// Test invalid settings
		cfg.PreparedStatementCacheSize = 0
		err := cfg.Validate()
		if err == nil {
			t.Error("Expected validation error for PreparedStatementCacheSize = 0")
		}
		if err != nil && !contains(err.Error(), "prepared_statement_cache_size must be positive") {
			t.Errorf("Expected error about prepared_statement_cache_size, got: %v", err)
		}

		cfg.PreparedStatementCacheSize = 1000
		cfg.PageSize = 0
		err = cfg.Validate()
		if err == nil {
			t.Error("Expected validation error for PageSize = 0")
		}
		if err != nil && !contains(err.Error(), "page_size must be positive") {
			t.Errorf("Expected error about page_size, got: %v", err)
		}

		// Test valid settings
		cfg.PageSize = 5000
		err = cfg.Validate()
		if err != nil {
			t.Errorf("Expected no validation error, got: %v", err)
		}
	})
}

func TestPreparedStatementCacheUnit(t *testing.T) {
	t.Run("cache_initialization", func(t *testing.T) {
		cache := &PreparedStatementCache{
			cache:   make(map[string]*gocql.Query),
			maxSize: 100,
		}

		if cache.cache == nil {
			t.Error("Expected non-nil cache")
		}
		if cache.maxSize != 100 {
			t.Errorf("Expected maxSize 100, got %d", cache.maxSize)
		}
	})

	t.Run("cache_clear", func(t *testing.T) {
		cache := &PreparedStatementCache{
			cache:   make(map[string]*gocql.Query),
			maxSize: 100,
		}

		// Add some dummy entries
		cache.cache["query1"] = nil
		cache.cache["query2"] = nil
		if len(cache.cache) != 2 {
			t.Errorf("Expected cache length 2, got %d", len(cache.cache))
		}

		cache.Clear()
		if len(cache.cache) != 0 {
			t.Errorf("Expected cache length 0 after clear, got %d", len(cache.cache))
		}
	})
}

func TestIdempotentQueryDetection(t *testing.T) {
	s := &ScyllaState{}

	tests := []struct {
		name       string
		cql        string
		idempotent bool
	}{
		{
			name:       "select_query",
			cql:        "SELECT * FROM pins_by_cid WHERE mh_prefix = ?",
			idempotent: true,
		},
		{
			name:       "delete_with_where",
			cql:        "DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?",
			idempotent: true,
		},
		{
			name:       "insert_if_not_exists",
			cql:        "INSERT INTO pins_by_cid (mh_prefix, cid_bin) VALUES (?, ?) IF NOT EXISTS",
			idempotent: true,
		},
		{
			name:       "update_with_where",
			cql:        "UPDATE pins_by_cid SET updated_at = ? WHERE mh_prefix = ?",
			idempotent: true,
		},
		{
			name:       "simple_insert",
			cql:        "INSERT INTO pins_by_cid (mh_prefix, cid_bin) VALUES (?, ?)",
			idempotent: false,
		},
		{
			name:       "truncate",
			cql:        "TRUNCATE pins_by_cid",
			idempotent: false,
		},
		{
			name:       "case_insensitive_select",
			cql:        "select * from pins_by_cid where mh_prefix = ?",
			idempotent: true,
		},
		{
			name:       "whitespace_handling",
			cql:        "  SELECT * FROM pins_by_cid  ",
			idempotent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.isIdempotentQuery(tt.cql)
			if result != tt.idempotent {
				t.Errorf("Query: %s - Expected idempotent %v, got %v", tt.cql, tt.idempotent, result)
			}
		})
	}
}

func TestConnectionOptimizationHelpers(t *testing.T) {
	t.Run("is_multi_dc", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()

		// Not multi-DC by default
		if cfg.IsMultiDC() {
			t.Error("Expected IsMultiDC to be false by default")
		}

		// Enable DC-aware routing
		cfg.DCAwareRouting = true
		cfg.LocalDC = "datacenter1"
		if !cfg.IsMultiDC() {
			t.Error("Expected IsMultiDC to be true with DC-aware routing and LocalDC set")
		}

		// DC-aware but no local DC
		cfg.LocalDC = ""
		if cfg.IsMultiDC() {
			t.Error("Expected IsMultiDC to be false without LocalDC")
		}
	})

	t.Run("connection_string", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()
		cfg.Hosts = []string{"host1", "host2"}
		cfg.Port = 9042

		connStr := cfg.GetConnectionString()
		if !contains(connStr, "host1:9042") {
			t.Errorf("Expected connection string to contain 'host1:9042', got: %s", connStr)
		}
		if !contains(connStr, "host2:9042") {
			t.Errorf("Expected connection string to contain 'host2:9042', got: %s", connStr)
		}

		// With TLS
		cfg.TLSEnabled = true
		connStr = cfg.GetConnectionString()
		if !contains(connStr, "(TLS)") {
			t.Errorf("Expected connection string to contain '(TLS)', got: %s", connStr)
		}

		// With multi-DC
		cfg.DCAwareRouting = true
		cfg.LocalDC = "datacenter1"
		connStr = cfg.GetConnectionString()
		if !contains(connStr, "(DC: datacenter1)") {
			t.Errorf("Expected connection string to contain '(DC: datacenter1)', got: %s", connStr)
		}
	})
}

func TestConnectionPoolDefaults(t *testing.T) {
	// Verify that our default values are reasonable for production use
	if DefaultMaxConnectionsPerHost != 20 {
		t.Errorf("Expected DefaultMaxConnectionsPerHost 20, got %d - should allow sufficient connections per host", DefaultMaxConnectionsPerHost)
	}
	if DefaultMinConnectionsPerHost != 2 {
		t.Errorf("Expected DefaultMinConnectionsPerHost 2, got %d - should maintain minimum connections", DefaultMinConnectionsPerHost)
	}
	if DefaultMaxWaitTime != 10*time.Second {
		t.Errorf("Expected DefaultMaxWaitTime 10s, got %v - should have reasonable wait time", DefaultMaxWaitTime)
	}
	if DefaultKeepAlive != 30*time.Second {
		t.Errorf("Expected DefaultKeepAlive 30s, got %v - should maintain connections", DefaultKeepAlive)
	}
	if DefaultPreparedStatementCacheSize != 1000 {
		t.Errorf("Expected DefaultPreparedStatementCacheSize 1000, got %d - should cache enough statements", DefaultPreparedStatementCacheSize)
	}
	if DefaultPageSize != 5000 {
		t.Errorf("Expected DefaultPageSize 5000, got %d - should use efficient page size", DefaultPageSize)
	}
	if DefaultReconnectInterval != 60*time.Second {
		t.Errorf("Expected DefaultReconnectInterval 60s, got %v - should reconnect reasonably", DefaultReconnectInterval)
	}
	if DefaultMaxReconnectAttempts != 3 {
		t.Errorf("Expected DefaultMaxReconnectAttempts 3, got %d - should retry connection attempts", DefaultMaxReconnectAttempts)
	}
}
