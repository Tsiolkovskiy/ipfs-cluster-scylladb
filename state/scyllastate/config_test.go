package scyllastate

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gocql/gocql"
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
	assert.Equal(t, "", cfg.Username)
	assert.Equal(t, "", cfg.Password)
	assert.False(t, cfg.TLSEnabled)
	assert.Equal(t, DefaultNumConns, cfg.NumConns)
	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultConnectTimeout, cfg.ConnectTimeout)
	assert.Equal(t, DefaultConsistency, cfg.Consistency)
	assert.Equal(t, DefaultSerialConsistency, cfg.SerialConsistency)
	assert.Equal(t, DefaultRetryNumRetries, cfg.RetryPolicy.NumRetries)
	assert.Equal(t, DefaultRetryMinDelay, cfg.RetryPolicy.MinRetryDelay)
	assert.Equal(t, DefaultRetryMaxDelay, cfg.RetryPolicy.MaxRetryDelay)
	assert.Equal(t, DefaultBatchSize, cfg.BatchSize)
	assert.Equal(t, DefaultBatchTimeout, cfg.BatchTimeout)
	assert.True(t, cfg.MetricsEnabled)
	assert.False(t, cfg.TracingEnabled)
	assert.False(t, cfg.DCAwareRouting)
	assert.False(t, cfg.TokenAwareRouting)
}

func TestConfig_ConfigKey(t *testing.T) {
	cfg := &Config{}
	assert.Equal(t, DefaultConfigKey, cfg.ConfigKey())
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid default config",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			wantErr: false,
		},
		{
			name: "empty hosts",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = []string{}
			},
			wantErr: true,
			errMsg:  "at least one host must be specified",
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
				cfg.Port = 70000
			},
			wantErr: true,
			errMsg:  "invalid port: 70000",
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
			name: "invalid num_conns",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.NumConns = 0
			},
			wantErr: true,
			errMsg:  "num_conns must be positive, got: 0",
		},
		{
			name: "invalid timeout",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Timeout = 0
			},
			wantErr: true,
			errMsg:  "timeout must be positive, got: 0s",
		},
		{
			name: "invalid consistency",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Consistency = "INVALID"
			},
			wantErr: true,
			errMsg:  "invalid consistency level: INVALID",
		},
		{
			name: "invalid serial consistency",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.SerialConsistency = "INVALID"
			},
			wantErr: true,
			errMsg:  "invalid serial consistency level: INVALID",
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
			name: "invalid batch size",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.BatchSize = 0
			},
			wantErr: true,
			errMsg:  "batch_size must be positive, got: 0",
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

func TestConfig_ApplyEnvVars(t *testing.T) {
	// Save original env vars
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

	// Set test env vars
	os.Setenv("SCYLLADB_HOSTS", "host1,host2,host3")
	os.Setenv("SCYLLADB_KEYSPACE", "test_keyspace")
	os.Setenv("SCYLLADB_USERNAME", "test_user")
	os.Setenv("SCYLLADB_PASSWORD", "test_pass")
	os.Setenv("SCYLLADB_TLS_ENABLED", "true")
	os.Setenv("SCYLLADB_LOCAL_DC", "dc1")

	cfg := &Config{}
	cfg.Default()
	err := cfg.ApplyEnvVars()
	require.NoError(t, err)

	assert.Equal(t, []string{"host1", "host2", "host3"}, cfg.Hosts)
	assert.Equal(t, "test_keyspace", cfg.Keyspace)
	assert.Equal(t, "test_user", cfg.Username)
	assert.Equal(t, "test_pass", cfg.Password)
	assert.True(t, cfg.TLSEnabled)
	assert.Equal(t, "dc1", cfg.LocalDC)
	assert.True(t, cfg.DCAwareRouting) // Should be enabled when LocalDC is set
}

func TestConfig_JSONSerialization(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Hosts = []string{"host1", "host2"}
	cfg.Username = "testuser"
	cfg.Password = "testpass"
	cfg.TLSEnabled = true

	// Test ToJSON
	jsonBytes, err := cfg.ToJSON()
	require.NoError(t, err)

	// Test LoadJSON
	cfg2 := &Config{}
	err = cfg2.LoadJSON(jsonBytes)
	require.NoError(t, err)

	assert.Equal(t, cfg.Hosts, cfg2.Hosts)
	assert.Equal(t, cfg.Port, cfg2.Port)
	assert.Equal(t, cfg.Keyspace, cfg2.Keyspace)
	assert.Equal(t, cfg.Username, cfg2.Username)
	assert.Equal(t, cfg.Password, cfg2.Password)
	assert.Equal(t, cfg.TLSEnabled, cfg2.TLSEnabled)
	assert.Equal(t, cfg.NumConns, cfg2.NumConns)
	assert.Equal(t, cfg.Timeout, cfg2.Timeout)
	assert.Equal(t, cfg.ConnectTimeout, cfg2.ConnectTimeout)
	assert.Equal(t, cfg.Consistency, cfg2.Consistency)
	assert.Equal(t, cfg.SerialConsistency, cfg2.SerialConsistency)
	assert.Equal(t, cfg.RetryPolicy, cfg2.RetryPolicy)
	assert.Equal(t, cfg.BatchSize, cfg2.BatchSize)
	assert.Equal(t, cfg.BatchTimeout, cfg2.BatchTimeout)
	assert.Equal(t, cfg.MetricsEnabled, cfg2.MetricsEnabled)
	assert.Equal(t, cfg.TracingEnabled, cfg2.TracingEnabled)
}

func TestConfig_ToDisplayJSON(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Password = "secret_password"

	displayJSON, err := cfg.ToDisplayJSON()
	require.NoError(t, err)

	// Parse the JSON to check password is hidden
	var displayData map[string]interface{}
	err = json.Unmarshal(displayJSON, &displayData)
	require.NoError(t, err)

	assert.Equal(t, "<hidden>", displayData["password"])
}

func TestConfig_GetConsistency(t *testing.T) {
	tests := []struct {
		level    string
		expected gocql.Consistency
	}{
		{"ANY", gocql.Any},
		{"ONE", gocql.One},
		{"TWO", gocql.Two},
		{"THREE", gocql.Three},
		{"QUORUM", gocql.Quorum},
		{"ALL", gocql.All},
		{"LOCAL_QUORUM", gocql.LocalQuorum},
		{"EACH_QUORUM", gocql.EachQuorum},
		{"LOCAL_ONE", gocql.LocalOne},
		{"INVALID", gocql.Quorum}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			cfg := &Config{Consistency: tt.level}
			assert.Equal(t, tt.expected, cfg.GetConsistency())
		})
	}
}

func TestConfig_GetSerialConsistency(t *testing.T) {
	tests := []struct {
		level    string
		expected gocql.SerialConsistency
	}{
		{"SERIAL", gocql.Serial},
		{"LOCAL_SERIAL", gocql.LocalSerial},
		{"INVALID", gocql.Serial}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			cfg := &Config{SerialConsistency: tt.level}
			assert.Equal(t, tt.expected, cfg.GetSerialConsistency())
		})
	}
}

func TestConfig_CreateAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantAuth bool
	}{
		{
			name:     "with credentials",
			username: "user",
			password: "pass",
			wantAuth: true,
		},
		{
			name:     "without credentials",
			username: "",
			password: "",
			wantAuth: false,
		},
		{
			name:     "username only",
			username: "user",
			password: "",
			wantAuth: false,
		},
		{
			name:     "password only",
			username: "",
			password: "pass",
			wantAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Username: tt.username,
				Password: tt.password,
			}

			auth := cfg.CreateAuthenticator()
			if tt.wantAuth {
				assert.NotNil(t, auth)
				passwordAuth, ok := auth.(gocql.PasswordAuthenticator)
				assert.True(t, ok)
				assert.Equal(t, tt.username, passwordAuth.Username)
				assert.Equal(t, tt.password, passwordAuth.Password)
			} else {
				assert.Nil(t, auth)
			}
		})
	}
}

func TestConfig_CreateClusterConfig(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Hosts = []string{"host1", "host2"}
	cfg.Username = "user"
	cfg.Password = "pass"
	cfg.LocalDC = "dc1"
	cfg.DCAwareRouting = true
	cfg.TokenAwareRouting = true

	cluster, err := cfg.CreateClusterConfig()
	require.NoError(t, err)

	assert.Equal(t, cfg.Hosts, cluster.Hosts)
	assert.Equal(t, cfg.Port, cluster.Port)
	assert.Equal(t, cfg.Keyspace, cluster.Keyspace)
	assert.Equal(t, cfg.NumConns, cluster.NumConns)
	assert.Equal(t, cfg.Timeout, cluster.Timeout)
	assert.Equal(t, cfg.ConnectTimeout, cluster.ConnectTimeout)
	assert.Equal(t, cfg.GetConsistency(), cluster.Consistency)
	assert.Equal(t, cfg.GetSerialConsistency(), cluster.SerialConsistency)
	assert.NotNil(t, cluster.Authenticator)
}

func TestConfig_LoadJSON_InvalidDuration(t *testing.T) {
	cfg := &Config{}

	invalidJSON := `{
		"hosts": ["localhost"],
		"timeout": "invalid_duration"
	}`

	err := cfg.LoadJSON([]byte(invalidJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout duration")
}

func TestConfig_LoadJSON_InvalidJSON(t *testing.T) {
	cfg := &Config{}

	invalidJSON := `{invalid json}`

	err := cfg.LoadJSON([]byte(invalidJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshaling ScyllaDB config")
}

func TestConfig_ValidateProduction(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid production config",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Hosts = []string{"host1", "host2", "host3"}
				cfg.Consistency = "QUORUM"
			},
			wantErr: false,
		},
		{
			name: "TLS not enabled",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
				cfg.Username = "user"
				cfg.Password = "pass"
			},
			wantErr: true,
			errMsg:  "TLS must be enabled in production",
		},
		{
			name: "no authentication",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = ""
				cfg.Password = ""
			},
			wantErr: true,
			errMsg:  "authentication credentials must be provided in production",
		},
		{
			name: "insecure TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
				cfg.Username = "user"
				cfg.Password = "pass"
			},
			wantErr: true,
			errMsg:  "TLS certificate verification cannot be disabled in production",
		},
		{
			name: "weak consistency",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "ANY"
			},
			wantErr: true,
			errMsg:  "consistency level ANY is not recommended for production",
		},
		{
			name: "insufficient hosts",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Hosts = []string{"host1"}
			},
			wantErr: true,
			errMsg:  "at least 3 hosts are recommended for production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			err := cfg.ValidateProduction()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetEffectiveConfig(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"SCYLLADB_HOSTS":    os.Getenv("SCYLLADB_HOSTS"),
		"SCYLLADB_USERNAME": os.Getenv("SCYLLADB_USERNAME"),
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

	// Set test env vars
	os.Setenv("SCYLLADB_HOSTS", "env-host1,env-host2")
	os.Setenv("SCYLLADB_USERNAME", "env-user")

	cfg := &Config{}
	cfg.Default()
	cfg.Hosts = []string{"config-host"}
	cfg.Username = "config-user"

	effectiveCfg, err := cfg.GetEffectiveConfig()
	require.NoError(t, err)

	// Original config should be unchanged
	assert.Equal(t, []string{"config-host"}, cfg.Hosts)
	assert.Equal(t, "config-user", cfg.Username)

	// Effective config should have env vars applied
	assert.Equal(t, []string{"env-host1", "env-host2"}, effectiveCfg.Hosts)
	assert.Equal(t, "env-user", effectiveCfg.Username)
}

func TestConfig_IsMultiDC(t *testing.T) {
	tests := []struct {
		name           string
		dcAwareRouting bool
		localDC        string
		expected       bool
	}{
		{
			name:           "multi-DC enabled",
			dcAwareRouting: true,
			localDC:        "dc1",
			expected:       true,
		},
		{
			name:           "DC aware but no local DC",
			dcAwareRouting: true,
			localDC:        "",
			expected:       false,
		},
		{
			name:           "local DC but not DC aware",
			dcAwareRouting: false,
			localDC:        "dc1",
			expected:       false,
		},
		{
			name:           "neither enabled",
			dcAwareRouting: false,
			localDC:        "",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DCAwareRouting: tt.dcAwareRouting,
				LocalDC:        tt.localDC,
			}
			assert.Equal(t, tt.expected, cfg.IsMultiDC())
		})
	}
}

func TestConfig_GetConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Config)
		expected string
	}{
		{
			name: "single host",
			setup: func(cfg *Config) {
				cfg.Hosts = []string{"localhost"}
				cfg.Port = 9042
			},
			expected: "localhost:9042",
		},
		{
			name: "multiple hosts",
			setup: func(cfg *Config) {
				cfg.Hosts = []string{"host1", "host2"}
				cfg.Port = 9042
			},
			expected: "host1:9042,host2:9042",
		},
		{
			name: "with TLS",
			setup: func(cfg *Config) {
				cfg.Hosts = []string{"localhost"}
				cfg.Port = 9042
				cfg.TLSEnabled = true
			},
			expected: "localhost:9042 (TLS)",
		},
		{
			name: "with multi-DC",
			setup: func(cfg *Config) {
				cfg.Hosts = []string{"localhost"}
				cfg.Port = 9042
				cfg.DCAwareRouting = true
				cfg.LocalDC = "dc1"
			},
			expected: "localhost:9042 (DC: dc1)",
		},
		{
			name: "with TLS and multi-DC",
			setup: func(cfg *Config) {
				cfg.Hosts = []string{"localhost"}
				cfg.Port = 9042
				cfg.TLSEnabled = true
				cfg.DCAwareRouting = true
				cfg.LocalDC = "dc1"
			},
			expected: "localhost:9042 (TLS) (DC: dc1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)
			assert.Equal(t, tt.expected, cfg.GetConnectionString())
		})
	}
}
