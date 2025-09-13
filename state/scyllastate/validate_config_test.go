package scyllastate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationOptions(t *testing.T) {
	t.Run("DefaultValidationOptions", func(t *testing.T) {
		opts := DefaultValidationOptions()
		assert.Equal(t, ValidationBasic, opts.Level)
		assert.False(t, opts.AllowInsecureTLS)
		assert.False(t, opts.AllowWeakConsistency)
		assert.Equal(t, 1, opts.MinHosts)
		assert.Equal(t, 100, opts.MaxHosts)
		assert.False(t, opts.RequireAuth)
	})

	t.Run("StrictValidationOptions", func(t *testing.T) {
		opts := StrictValidationOptions()
		assert.Equal(t, ValidationStrict, opts.Level)
		assert.False(t, opts.AllowInsecureTLS)
		assert.False(t, opts.AllowWeakConsistency)
		assert.Equal(t, 3, opts.MinHosts)
		assert.Equal(t, 50, opts.MaxHosts)
		assert.True(t, opts.RequireAuth)
	})

	t.Run("DevelopmentValidationOptions", func(t *testing.T) {
		opts := DevelopmentValidationOptions()
		assert.Equal(t, ValidationDevelopment, opts.Level)
		assert.True(t, opts.AllowInsecureTLS)
		assert.True(t, opts.AllowWeakConsistency)
		assert.Equal(t, 1, opts.MinHosts)
		assert.Equal(t, 10, opts.MaxHosts)
		assert.False(t, opts.RequireAuth)
	})
}

func TestConfig_ValidateWithOptions(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		opts    *ValidationOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid basic config",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			opts:    DefaultValidationOptions(),
			wantErr: false,
		},
		{
			name: "strict validation - missing TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
			},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "TLS must be enabled in strict validation mode",
		},
		{
			name: "strict validation - missing auth",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = ""
				cfg.Password = ""
			},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "authentication credentials are required in strict validation mode",
		},
		{
			name: "strict validation - insecure TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
				cfg.Username = "user"
				cfg.Password = "pass"
			},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "insecure TLS verification is not allowed in strict validation mode",
		},
		{
			name: "strict validation - weak consistency",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "ANY"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "consistency level ANY is not recommended for production",
		},
		{
			name: "strict validation - insufficient hosts",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Hosts = []string{"host1"}
			},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "at least 3 hosts are required",
		},
		{
			name: "development validation - allows weak settings",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
				cfg.Consistency = "ANY"
			},
			opts:    DevelopmentValidationOptions(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			err := cfg.ValidateWithOptions(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_validateHosts(t *testing.T) {
	tests := []struct {
		name    string
		hosts   []string
		opts    *ValidationOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid single host",
			hosts:   []string{"localhost"},
			opts:    DefaultValidationOptions(),
			wantErr: false,
		},
		{
			name:    "valid multiple hosts",
			hosts:   []string{"host1", "host2", "host3"},
			opts:    DefaultValidationOptions(),
			wantErr: false,
		},
		{
			name:    "valid IP addresses",
			hosts:   []string{"127.0.0.1", "192.168.1.1"},
			opts:    DefaultValidationOptions(),
			wantErr: false,
		},
		{
			name:    "empty hosts",
			hosts:   []string{},
			opts:    DefaultValidationOptions(),
			wantErr: true,
			errMsg:  "at least one host must be specified",
		},
		{
			name:    "duplicate hosts",
			hosts:   []string{"host1", "host1"},
			opts:    DefaultValidationOptions(),
			wantErr: true,
			errMsg:  "duplicate host found: host1",
		},
		{
			name:    "too few hosts for strict validation",
			hosts:   []string{"host1"},
			opts:    StrictValidationOptions(),
			wantErr: true,
			errMsg:  "at least 3 hosts are required",
		},
		{
			name:    "empty host",
			hosts:   []string{"host1", ""},
			opts:    DefaultValidationOptions(),
			wantErr: true,
			errMsg:  "host cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Hosts: tt.hosts}
			err := cfg.validateHosts(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_validateHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid hostname",
			host:    "example.com",
			wantErr: false,
		},
		{
			name:    "valid IPv4",
			host:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			host:    "::1",
			wantErr: false,
		},
		{
			name:    "valid complex hostname",
			host:    "scylla-node-1.datacenter-1.example.com",
			wantErr: false,
		},
		{
			name:    "empty host",
			host:    "",
			wantErr: true,
			errMsg:  "host cannot be empty",
		},
		{
			name:    "hostname starting with dot",
			host:    ".example.com",
			wantErr: true,
			errMsg:  "hostname cannot start or end with a dot",
		},
		{
			name:    "hostname ending with dot",
			host:    "example.com.",
			wantErr: true,
			errMsg:  "hostname cannot start or end with a dot",
		},
		{
			name:    "hostname too long",
			host:    "a" + string(make([]byte, 250)) + ".com",
			wantErr: true,
			errMsg:  "hostname too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			err := cfg.validateHost(tt.host)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_validateKeyspace(t *testing.T) {
	tests := []struct {
		name     string
		keyspace string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid keyspace",
			keyspace: "ipfs_pins",
			wantErr:  false,
		},
		{
			name:     "valid keyspace with underscore",
			keyspace: "_test_keyspace",
			wantErr:  false,
		},
		{
			name:     "valid keyspace with numbers",
			keyspace: "keyspace123",
			wantErr:  false,
		},
		{
			name:     "empty keyspace",
			keyspace: "",
			wantErr:  true,
			errMsg:   "keyspace cannot be empty",
		},
		{
			name:     "keyspace starting with number",
			keyspace: "123keyspace",
			wantErr:  true,
			errMsg:   "keyspace name must start with a letter or underscore",
		},
		{
			name:     "keyspace with invalid character",
			keyspace: "keyspace-test",
			wantErr:  true,
			errMsg:   "invalid character in keyspace name",
		},
		{
			name:     "reserved keyspace",
			keyspace: "system",
			wantErr:  true,
			errMsg:   "keyspace name 'system' is reserved",
		},
		{
			name:     "reserved keyspace case insensitive",
			keyspace: "SYSTEM",
			wantErr:  true,
			errMsg:   "keyspace name 'SYSTEM' is reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Keyspace: tt.keyspace}
			err := cfg.validateKeyspace()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_validateTimeouts(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		connectTimeout time.Duration
		batchTimeout   time.Duration
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "valid timeouts",
			timeout:        30 * time.Second,
			connectTimeout: 10 * time.Second,
			batchTimeout:   1 * time.Second,
			wantErr:        false,
		},
		{
			name:           "zero timeout",
			timeout:        0,
			connectTimeout: 10 * time.Second,
			batchTimeout:   1 * time.Second,
			wantErr:        true,
			errMsg:         "timeout must be positive",
		},
		{
			name:           "zero connect timeout",
			timeout:        30 * time.Second,
			connectTimeout: 0,
			batchTimeout:   1 * time.Second,
			wantErr:        true,
			errMsg:         "connect_timeout must be positive",
		},
		{
			name:           "zero batch timeout",
			timeout:        30 * time.Second,
			connectTimeout: 10 * time.Second,
			batchTimeout:   0,
			wantErr:        true,
			errMsg:         "batch_timeout must be positive",
		},
		{
			name:           "connect timeout greater than timeout",
			timeout:        10 * time.Second,
			connectTimeout: 30 * time.Second,
			batchTimeout:   1 * time.Second,
			wantErr:        true,
			errMsg:         "connect_timeout (30s) should not be greater than timeout (10s)",
		},
		{
			name:           "timeout too large",
			timeout:        10 * time.Minute,
			connectTimeout: 1 * time.Second,
			batchTimeout:   1 * time.Second,
			wantErr:        true,
			errMsg:         "timeout too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Timeout:        tt.timeout,
				ConnectTimeout: tt.connectTimeout,
				BatchTimeout:   tt.batchTimeout,
			}
			err := cfg.validateTimeouts()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetValidationSummary(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.TLSEnabled = true
	cfg.Username = "user"
	cfg.Password = "pass"
	cfg.Hosts = []string{"host1", "host2", "host3"}
	cfg.TokenAwareRouting = true

	summary := cfg.GetValidationSummary()

	assert.Equal(t, 3, summary["hosts_count"])
	assert.Equal(t, true, summary["tls_enabled"])
	assert.Equal(t, true, summary["auth_configured"])
	assert.Equal(t, false, summary["multi_dc"])
	assert.Equal(t, "QUORUM", summary["consistency_level"])
	assert.Equal(t, "SERIAL", summary["serial_consistency_level"])
	assert.Equal(t, DefaultNumConns, summary["connection_pool_size"])
	assert.Equal(t, DefaultTimeout.String(), summary["timeout"])
	assert.Equal(t, DefaultBatchSize, summary["batch_size"])
	assert.Equal(t, true, summary["metrics_enabled"])

	// Check security score
	securityScore, ok := summary["security_score"].(int)
	assert.True(t, ok)
	assert.Greater(t, securityScore, 80) // Should be high with TLS, auth, good consistency, and multiple hosts

	// Check performance score
	perfScore, ok := summary["performance_score"].(int)
	assert.True(t, ok)
	assert.Greater(t, perfScore, 75) // Should be high with token-aware routing and good settings

	// Check that issues arrays exist
	_, ok = summary["security_issues"].([]string)
	assert.True(t, ok)
	_, ok = summary["performance_issues"].([]string)
	assert.True(t, ok)
}

func TestConfig_ValidateAndSummarize(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()

		summary, err := cfg.ValidateAndSummarize(DefaultValidationOptions())
		require.NoError(t, err)

		assert.Equal(t, true, summary["validation_passed"])
		_, hasError := summary["validation_error"]
		assert.False(t, hasError)
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &Config{}
		cfg.Default()
		cfg.Hosts = []string{} // Invalid: no hosts

		summary, err := cfg.ValidateAndSummarize(DefaultValidationOptions())
		require.Error(t, err)

		assert.Equal(t, false, summary["validation_passed"])
		errorMsg, hasError := summary["validation_error"].(string)
		assert.True(t, hasError)
		assert.Contains(t, errorMsg, "at least one host must be specified")
	})
}

func TestConfig_validateBatchSettings(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid batch size",
			batchSize: 1000,
			wantErr:   false,
		},
		{
			name:      "zero batch size",
			batchSize: 0,
			wantErr:   true,
			errMsg:    "batch_size must be positive",
		},
		{
			name:      "negative batch size",
			batchSize: -1,
			wantErr:   true,
			errMsg:    "batch_size must be positive",
		},
		{
			name:      "batch size too large",
			batchSize: 20000,
			wantErr:   true,
			errMsg:    "batch_size too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{BatchSize: tt.batchSize}
			err := cfg.validateBatchSettings()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_validateTLSSettings(t *testing.T) {
	tests := []struct {
		name       string
		tlsEnabled bool
		certFile   string
		keyFile    string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "TLS disabled",
			tlsEnabled: false,
			wantErr:    false,
		},
		{
			name:       "TLS enabled without files",
			tlsEnabled: true,
			wantErr:    false,
		},
		{
			name:       "TLS with both cert and key",
			tlsEnabled: true,
			certFile:   "/path/to/cert.pem",
			keyFile:    "/path/to/key.pem",
			wantErr:    false,
		},
		{
			name:       "TLS with same cert and key file",
			tlsEnabled: true,
			certFile:   "/path/to/same.pem",
			keyFile:    "/path/to/same.pem",
			wantErr:    true,
			errMsg:     "TLS cert file and key file cannot be the same",
		},
		{
			name:       "TLS with only cert file",
			tlsEnabled: true,
			certFile:   "/path/to/cert.pem",
			keyFile:    "",
			wantErr:    true,
			errMsg:     "both TLS cert file and key file must be specified together",
		},
		{
			name:       "TLS with only key file",
			tlsEnabled: true,
			certFile:   "",
			keyFile:    "/path/to/key.pem",
			wantErr:    true,
			errMsg:     "both TLS cert file and key file must be specified together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				TLSEnabled:  tt.tlsEnabled,
				TLSCertFile: tt.certFile,
				TLSKeyFile:  tt.keyFile,
			}
			err := cfg.validateTLSSettings(DefaultValidationOptions())
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
