package scyllastate

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ValidationComprehensive(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			wantErr: false,
		},
		{
			name: "valid production config",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = []string{"host1", "host2", "host3"}
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.LocalDC = "dc1"
				cfg.DCAwareRouting = true
				cfg.TokenAwareRouting = true
			},
			wantErr: false,
		},
		{
			name: "empty hosts array",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = []string{}
			},
			wantErr: true,
			errMsg:  "at least one host must be specified",
		},
		{
			name: "nil hosts array",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = nil
			},
			wantErr: true,
			errMsg:  "at least one host must be specified",
		},
		{
			name: "invalid port - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Port = -1
			},
			wantErr: true,
			errMsg:  "invalid port: -1",
		},
		{
			name: "invalid port - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Port = 0
			},
			wantErr: true,
			errMsg:  "invalid port: 0",
		},
		{
			name: "invalid port - too high",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Port = 65536
			},
			wantErr: true,
			errMsg:  "invalid port: 65536",
		},
		{
			name: "empty keyspace",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Keyspace = ""
			},
			wantErr: true,
			errMsg:  "keyspace cannot be empty",
		},
		{
			name: "whitespace-only keyspace",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Keyspace = "   "
			},
			wantErr: false, // Whitespace keyspace is technically valid
		},
		{
			name: "invalid num_conns - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.NumConns = 0
			},
			wantErr: true,
			errMsg:  "num_conns must be positive, got: 0",
		},
		{
			name: "invalid num_conns - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.NumConns = -5
			},
			wantErr: true,
			errMsg:  "num_conns must be positive, got: -5",
		},
		{
			name: "invalid timeout - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Timeout = 0
			},
			wantErr: true,
			errMsg:  "timeout must be positive, got: 0s",
		},
		{
			name: "invalid timeout - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Timeout = -5 * time.Second
			},
			wantErr: true,
			errMsg:  "timeout must be positive, got: -5s",
		},
		{
			name: "invalid connect_timeout - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.ConnectTimeout = 0
			},
			wantErr: true,
			errMsg:  "connect_timeout must be positive, got: 0s",
		},
		{
			name: "invalid consistency level",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Consistency = "INVALID_LEVEL"
			},
			wantErr: true,
			errMsg:  "invalid consistency level: INVALID_LEVEL",
		},
		{
			name: "case sensitive consistency level",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Consistency = "quorum" // lowercase
			},
			wantErr: true,
			errMsg:  "invalid consistency level: quorum",
		},
		{
			name: "invalid serial consistency level",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.SerialConsistency = "INVALID_SERIAL"
			},
			wantErr: true,
			errMsg:  "invalid serial consistency level: INVALID_SERIAL",
		},
		{
			name: "valid consistency levels",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Consistency = "LOCAL_QUORUM"
				cfg.SerialConsistency = "LOCAL_SERIAL"
			},
			wantErr: false,
		},
		{
			name: "invalid retry policy - negative retries",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.NumRetries = -1
			},
			wantErr: true,
			errMsg:  "retry num_retries must be non-negative, got: -1",
		},
		{
			name: "invalid retry policy - zero min delay",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.MinRetryDelay = 0
			},
			wantErr: true,
			errMsg:  "retry min_retry_delay must be positive, got: 0s",
		},
		{
			name: "invalid retry policy - negative min delay",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.MinRetryDelay = -100 * time.Millisecond
			},
			wantErr: true,
			errMsg:  "retry min_retry_delay must be positive, got: -100ms",
		},
		{
			name: "invalid retry policy - zero max delay",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.MaxRetryDelay = 0
			},
			wantErr: true,
			errMsg:  "retry max_retry_delay must be positive, got: 0s",
		},
		{
			name: "invalid retry policy - min > max delay",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.MinRetryDelay = 10 * time.Second
				cfg.RetryPolicy.MaxRetryDelay = 5 * time.Second
			},
			wantErr: true,
			errMsg:  "retry min_retry_delay (10s) cannot be greater than max_retry_delay (5s)",
		},
		{
			name: "valid retry policy - equal min and max delay",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.RetryPolicy.MinRetryDelay = 5 * time.Second
				cfg.RetryPolicy.MaxRetryDelay = 5 * time.Second
			},
			wantErr: false,
		},
		{
			name: "invalid batch size - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.BatchSize = 0
			},
			wantErr: true,
			errMsg:  "batch_size must be positive, got: 0",
		},
		{
			name: "invalid batch size - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.BatchSize = -100
			},
			wantErr: true,
			errMsg:  "batch_size must be positive, got: -100",
		},
		{
			name: "invalid batch timeout - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.BatchTimeout = 0
			},
			wantErr: true,
			errMsg:  "batch_timeout must be positive, got: 0s",
		},
		{
			name: "invalid batch timeout - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.BatchTimeout = -1 * time.Second
			},
			wantErr: true,
			errMsg:  "batch_timeout must be positive, got: -1s",
		},
		{
			name: "invalid connection pool - zero max connections",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MaxConnectionsPerHost = 0
			},
			wantErr: true,
			errMsg:  "max_connections_per_host must be positive, got: 0",
		},
		{
			name: "invalid connection pool - zero min connections",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MinConnectionsPerHost = 0
			},
			wantErr: true,
			errMsg:  "min_connections_per_host must be positive, got: 0",
		},
		{
			name: "invalid connection pool - min > max connections",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MinConnectionsPerHost = 10
				cfg.MaxConnectionsPerHost = 5
			},
			wantErr: true,
			errMsg:  "min_connections_per_host (10) cannot be greater than max_connections_per_host (5)",
		},
		{
			name: "invalid max wait time - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MaxWaitTime = 0
			},
			wantErr: true,
			errMsg:  "max_wait_time must be positive, got: 0s",
		},
		{
			name: "invalid keep alive - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.KeepAlive = 0
			},
			wantErr: true,
			errMsg:  "keep_alive must be positive, got: 0s",
		},
		{
			name: "invalid reconnect interval - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.ReconnectInterval = 0
			},
			wantErr: true,
			errMsg:  "reconnect_interval must be positive, got: 0s",
		},
		{
			name: "invalid max reconnect attempts - negative",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MaxReconnectAttempts = -1
			},
			wantErr: true,
			errMsg:  "max_reconnect_attempts must be non-negative, got: -1",
		},
		{
			name: "valid max reconnect attempts - zero (unlimited)",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.MaxReconnectAttempts = 0
			},
			wantErr: false,
		},
		{
			name: "invalid prepared statement cache size - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.PreparedStatementCacheSize = 0
			},
			wantErr: true,
			errMsg:  "prepared_statement_cache_size must be positive, got: 0",
		},
		{
			name: "invalid page size - zero",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.PageSize = 0
			},
			wantErr: true,
			errMsg:  "page_size must be positive, got: 0",
		},
		{
			name: "large page size - should be valid but might log warning",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.PageSize = 200000 // Very large page size
			},
			wantErr: false, // Should be valid, just potentially inefficient
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_TLSValidation(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := ioutil.TempDir("", "scylla_tls_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create valid certificate files for testing
	certFile := filepath.Join(tempDir, "cert.pem")
	keyFile := filepath.Join(tempDir, "key.pem")
	caFile := filepath.Join(tempDir, "ca.pem")

	// Write sample certificate content (this is a dummy cert for testing)
	certContent := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAMlyFqk69v+9MA0GCSqGSIb3DQEBCwUAMBQxEjAQBgNVBAMMCWxv
Y2FsaG9zdDAeFw0yMzEwMTAwMDAwMDBaFw0yNDEwMDkwMDAwMDBaMBQxEjAQBgNV
BAMMCWxvY2FsaG9zdDBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQDTgvwjlRHZ9kz+
P/4q9Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+
8Z9+8Z9+AgMBAAEwDQYJKoZIhvcNAQELBQADQQBQZ9+8Z9+8Z9+8Z9+8Z9+8Z9+8
Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8
-----END CERTIFICATE-----`

	keyContent := `-----BEGIN PRIVATE KEY-----
MIIBVAIBADANBgkqhkiG9w0BAQEFAASCAT4wggE6AgEAAkEA04L8I5UR2fZM/j/+
KvWffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGf
fvGffvGffvIDAQABAkEAz9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8
Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8wIhAOZ9+8Z9+8Z9+8Z9+8Z9+
8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8AiEA5n37xn37xn37xn37xn37xn37xn37xn37xn3
7xn37xn37xn3wIgDmffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGffvGff
vAIhAOZ9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8Z9+8AiEA5n37xn37x
n37xn37xn37xn37xn37xn37xn37xn37xn37xn3w=
-----END PRIVATE KEY-----`

	err = ioutil.WriteFile(certFile, []byte(certContent), 0644)
	require.NoError(t, err)
	err = ioutil.WriteFile(keyFile, []byte(keyContent), 0644)
	require.NoError(t, err)
	err = ioutil.WriteFile(caFile, []byte(certContent), 0644) // Use same content for CA
	require.NoError(t, err)

	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "TLS disabled - should be valid",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
			},
			wantErr: false,
		},
		{
			name: "TLS enabled without cert files - should be valid",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
			},
			wantErr: false,
		},
		{
			name: "TLS enabled with valid cert files",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
			},
			wantErr: false,
		},
		{
			name: "TLS enabled with valid cert files and CA",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
			},
			wantErr: false,
		},
		{
			name: "TLS enabled with non-existent cert file",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = "/non/existent/cert.pem"
				cfg.TLSKeyFile = keyFile
			},
			wantErr: true,
			errMsg:  "TLS cert file does not exist",
		},
		{
			name: "TLS enabled with non-existent key file",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = "/non/existent/key.pem"
			},
			wantErr: true,
			errMsg:  "TLS key file does not exist",
		},
		{
			name: "TLS enabled with non-existent CA file",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCAFile = "/non/existent/ca.pem"
			},
			wantErr: true,
			errMsg:  "TLS CA file does not exist",
		},
		{
			name: "TLS enabled with cert file but no key file",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = ""
			},
			wantErr: false, // Should be valid - cert/key are optional
		},
		{
			name: "TLS enabled with key file but no cert file",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = ""
				cfg.TLSKeyFile = keyFile
			},
			wantErr: false, // Should be valid - cert/key are optional
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_JSONValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid JSON",
			json: `{
				"hosts": ["localhost"],
				"port": 9042,
				"keyspace": "test",
				"timeout": "30s",
				"connect_timeout": "10s",
				"batch_timeout": "1s",
				"retry_policy": {
					"num_retries": 3,
					"min_retry_delay": "100ms",
					"max_retry_delay": "10s"
				}
			}`,
			wantErr: false,
		},
		{
			name: "invalid JSON syntax",
			json: `{
				"hosts": ["localhost",
				"port": 9042
			}`,
			wantErr: true,
			errMsg:  "error unmarshaling ScyllaDB config",
		},
		{
			name: "invalid timeout duration",
			json: `{
				"hosts": ["localhost"],
				"timeout": "invalid_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid timeout duration",
		},
		{
			name: "invalid connect_timeout duration",
			json: `{
				"hosts": ["localhost"],
				"connect_timeout": "not_a_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid connect_timeout duration",
		},
		{
			name: "invalid batch_timeout duration",
			json: `{
				"hosts": ["localhost"],
				"batch_timeout": "bad_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid batch_timeout duration",
		},
		{
			name: "invalid retry min_retry_delay duration",
			json: `{
				"hosts": ["localhost"],
				"retry_policy": {
					"min_retry_delay": "invalid"
				}
			}`,
			wantErr: true,
			errMsg:  "invalid retry min_retry_delay duration",
		},
		{
			name: "invalid retry max_retry_delay duration",
			json: `{
				"hosts": ["localhost"],
				"retry_policy": {
					"max_retry_delay": "bad"
				}
			}`,
			wantErr: true,
			errMsg:  "invalid retry max_retry_delay duration",
		},
		{
			name: "invalid max_connections_per_host",
			json: `{
				"hosts": ["localhost"],
				"max_connections_per_host": "not_a_number"
			}`,
			wantErr: true,
			errMsg:  "invalid max_connections_per_host",
		},
		{
			name: "invalid min_connections_per_host",
			json: `{
				"hosts": ["localhost"],
				"min_connections_per_host": "not_a_number"
			}`,
			wantErr: true,
			errMsg:  "invalid min_connections_per_host",
		},
		{
			name: "invalid max_wait_time duration",
			json: `{
				"hosts": ["localhost"],
				"max_wait_time": "invalid_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid max_wait_time duration",
		},
		{
			name: "invalid keep_alive duration",
			json: `{
				"hosts": ["localhost"],
				"keep_alive": "bad_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid keep_alive duration",
		},
		{
			name: "invalid prepared_statement_cache_size",
			json: `{
				"hosts": ["localhost"],
				"prepared_statement_cache_size": "not_a_number"
			}`,
			wantErr: true,
			errMsg:  "invalid prepared_statement_cache_size",
		},
		{
			name: "invalid page_size",
			json: `{
				"hosts": ["localhost"],
				"page_size": "not_a_number"
			}`,
			wantErr: true,
			errMsg:  "invalid page_size",
		},
		{
			name: "invalid reconnect_interval duration",
			json: `{
				"hosts": ["localhost"],
				"reconnect_interval": "bad_duration"
			}`,
			wantErr: true,
			errMsg:  "invalid reconnect_interval duration",
		},
		{
			name: "invalid max_reconnect_attempts",
			json: `{
				"hosts": ["localhost"],
				"max_reconnect_attempts": "not_a_number"
			}`,
			wantErr: true,
			errMsg:  "invalid max_reconnect_attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			err := cfg.LoadJSON([]byte(tt.json))

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_EnvironmentVariableValidation(t *testing.T) {
	// Save original environment variables
	originalVars := map[string]string{
		"SCYLLADB_HOSTS":       os.Getenv("SCYLLADB_HOSTS"),
		"SCYLLADB_KEYSPACE":    os.Getenv("SCYLLADB_KEYSPACE"),
		"SCYLLADB_USERNAME":    os.Getenv("SCYLLADB_USERNAME"),
		"SCYLLADB_PASSWORD":    os.Getenv("SCYLLADB_PASSWORD"),
		"SCYLLADB_TLS_ENABLED": os.Getenv("SCYLLADB_TLS_ENABLED"),
		"SCYLLADB_LOCAL_DC":    os.Getenv("SCYLLADB_LOCAL_DC"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		check   func(*testing.T, *Config)
	}{
		{
			name: "valid environment variables",
			envVars: map[string]string{
				"SCYLLADB_HOSTS":       "host1,host2,host3",
				"SCYLLADB_KEYSPACE":    "test_keyspace",
				"SCYLLADB_USERNAME":    "test_user",
				"SCYLLADB_PASSWORD":    "test_pass",
				"SCYLLADB_TLS_ENABLED": "true",
				"SCYLLADB_LOCAL_DC":    "dc1",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"host1", "host2", "host3"}, cfg.Hosts)
				assert.Equal(t, "test_keyspace", cfg.Keyspace)
				assert.Equal(t, "test_user", cfg.Username)
				assert.Equal(t, "test_pass", cfg.Password)
				assert.True(t, cfg.TLSEnabled)
				assert.Equal(t, "dc1", cfg.LocalDC)
				assert.True(t, cfg.DCAwareRouting) // Should be enabled when LocalDC is set
			},
		},
		{
			name: "TLS disabled via environment",
			envVars: map[string]string{
				"SCYLLADB_TLS_ENABLED": "false",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.False(t, cfg.TLSEnabled)
			},
		},
		{
			name: "TLS enabled with case variations",
			envVars: map[string]string{
				"SCYLLADB_TLS_ENABLED": "TRUE",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TLSEnabled)
			},
		},
		{
			name: "hosts with extra whitespace",
			envVars: map[string]string{
				"SCYLLADB_HOSTS": " host1 , host2 , host3 ",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"host1", "host2", "host3"}, cfg.Hosts)
			},
		},
		{
			name: "single host",
			envVars: map[string]string{
				"SCYLLADB_HOSTS": "single-host",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"single-host"}, cfg.Hosts)
			},
		},
		{
			name: "empty environment variables should not override defaults",
			envVars: map[string]string{
				"SCYLLADB_HOSTS":    "",
				"SCYLLADB_KEYSPACE": "",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				// Should keep default values when env vars are empty
				assert.Equal(t, []string{"127.0.0.1"}, cfg.Hosts)
				assert.Equal(t, DefaultKeyspace, cfg.Keyspace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all test environment variables first
			for key := range originalVars {
				os.Unsetenv(key)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg := &Config{}
			cfg.Default()
			err := cfg.ApplyEnvVars()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, cfg)
				}
			}
		})
	}
}

func TestConfig_RoundTripSerialization(t *testing.T) {
	// Test that configuration can be serialized to JSON and back without loss
	original := &Config{}
	original.Default()
	original.Hosts = []string{"host1", "host2", "host3"}
	original.Username = "testuser"
	original.Password = "testpass"
	original.TLSEnabled = true
	original.TLSCertFile = "/path/to/cert.pem"
	original.TLSKeyFile = "/path/to/key.pem"
	original.TLSCAFile = "/path/to/ca.pem"
	original.LocalDC = "dc1"
	original.DCAwareRouting = true
	original.TokenAwareRouting = true
	original.RetryPolicy.NumRetries = 5
	original.RetryPolicy.MinRetryDelay = 200 * time.Millisecond
	original.RetryPolicy.MaxRetryDelay = 30 * time.Second

	// Serialize to JSON
	jsonData, err := original.ToJSON()
	require.NoError(t, err)

	// Deserialize from JSON
	restored := &Config{}
	err = restored.LoadJSON(jsonData)
	require.NoError(t, err)

	// Compare all fields
	assert.Equal(t, original.Hosts, restored.Hosts)
	assert.Equal(t, original.Port, restored.Port)
	assert.Equal(t, original.Keyspace, restored.Keyspace)
	assert.Equal(t, original.Username, restored.Username)
	assert.Equal(t, original.Password, restored.Password)
	assert.Equal(t, original.TLSEnabled, restored.TLSEnabled)
	assert.Equal(t, original.TLSCertFile, restored.TLSCertFile)
	assert.Equal(t, original.TLSKeyFile, restored.TLSKeyFile)
	assert.Equal(t, original.TLSCAFile, restored.TLSCAFile)
	assert.Equal(t, original.TLSInsecureSkipVerify, restored.TLSInsecureSkipVerify)
	assert.Equal(t, original.NumConns, restored.NumConns)
	assert.Equal(t, original.Timeout, restored.Timeout)
	assert.Equal(t, original.ConnectTimeout, restored.ConnectTimeout)
	assert.Equal(t, original.Consistency, restored.Consistency)
	assert.Equal(t, original.SerialConsistency, restored.SerialConsistency)
	assert.Equal(t, original.RetryPolicy, restored.RetryPolicy)
	assert.Equal(t, original.BatchSize, restored.BatchSize)
	assert.Equal(t, original.BatchTimeout, restored.BatchTimeout)
	assert.Equal(t, original.MetricsEnabled, restored.MetricsEnabled)
	assert.Equal(t, original.TracingEnabled, restored.TracingEnabled)
	assert.Equal(t, original.LocalDC, restored.LocalDC)
	assert.Equal(t, original.DCAwareRouting, restored.DCAwareRouting)
	assert.Equal(t, original.TokenAwareRouting, restored.TokenAwareRouting)
}

func TestConfig_DisplayJSONHidesPassword(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Password = "super_secret_password"

	displayJSON, err := cfg.ToDisplayJSON()
	require.NoError(t, err)

	// Parse the JSON to verify password is hidden
	var displayData map[string]interface{}
	err = json.Unmarshal(displayJSON, &displayData)
	require.NoError(t, err)

	assert.Equal(t, "<hidden>", displayData["password"])
	assert.NotEqual(t, "super_secret_password", displayData["password"])
}

func TestConfig_DisplayJSONEmptyPassword(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Password = "" // Empty password

	displayJSON, err := cfg.ToDisplayJSON()
	require.NoError(t, err)

	// Parse the JSON to verify empty password is shown as empty
	var displayData map[string]interface{}
	err = json.Unmarshal(displayJSON, &displayData)
	require.NoError(t, err)

	// When password is empty, it should remain empty in display JSON
	assert.Equal(t, "", displayData["password"])
}
