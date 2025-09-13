package scyllastate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper functions for creating test certificates
func createTestCertificates(t *testing.T, tempDir string) (certFile, keyFile, caFile string) {
	// Generate CA private key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Generate client private key
	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create client certificate template
	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{"Test Client"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Parse CA certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	// Create client certificate signed by CA
	clientCertDER, err := x509.CreateCertificate(rand.Reader, &clientTemplate, caCert, &clientPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Write CA certificate to file
	caFile = filepath.Join(tempDir, "ca.pem")
	caPEMFile, err := os.Create(caFile)
	require.NoError(t, err)
	defer caPEMFile.Close()

	err = pem.Encode(caPEMFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})
	require.NoError(t, err)

	// Write client certificate to file
	certFile = filepath.Join(tempDir, "client.pem")
	certPEMFile, err := os.Create(certFile)
	require.NoError(t, err)
	defer certPEMFile.Close()

	err = pem.Encode(certPEMFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: clientCertDER,
	})
	require.NoError(t, err)

	// Write client private key to file
	keyFile = filepath.Join(tempDir, "client-key.pem")
	keyPEMFile, err := os.Create(keyFile)
	require.NoError(t, err)
	defer keyPEMFile.Close()

	err = pem.Encode(keyPEMFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(clientPrivKey),
	})
	require.NoError(t, err)

	return certFile, keyFile, caFile
}

func TestConfig_CreateTLSConfig(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		setup    func(*Config)
		wantNil  bool
		wantErr  bool
		errMsg   string
		validate func(*testing.T, *tls.Config)
	}{
		{
			name: "TLS disabled",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = false
			},
			wantNil: true,
			wantErr: false,
		},
		{
			name: "TLS enabled without certificates",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = false
			},
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				assert.False(t, tlsConfig.InsecureSkipVerify)
				assert.Empty(t, tlsConfig.Certificates)
				assert.Nil(t, tlsConfig.RootCAs)
			},
		},
		{
			name: "TLS enabled with insecure skip verify",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
			},
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				assert.True(t, tlsConfig.InsecureSkipVerify)
			},
		},
		{
			name: "TLS enabled with valid certificates",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
			},
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				assert.False(t, tlsConfig.InsecureSkipVerify)
				assert.Len(t, tlsConfig.Certificates, 1)
				assert.NotNil(t, tlsConfig.RootCAs)
			},
		},
		{
			name: "TLS enabled with only client certificate",
			setup: func(cfg *Config) {
				certFile, keyFile, _ := createTestCertificates(t, tempDir)
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSInsecureSkipVerify = false
			},
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				assert.False(t, tlsConfig.InsecureSkipVerify)
				assert.Len(t, tlsConfig.Certificates, 1)
				assert.Nil(t, tlsConfig.RootCAs)
			},
		},
		{
			name: "TLS enabled with only CA certificate",
			setup: func(cfg *Config) {
				_, _, caFile := createTestCertificates(t, tempDir)
				cfg.TLSEnabled = true
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
			},
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				assert.False(t, tlsConfig.InsecureSkipVerify)
				assert.Empty(t, tlsConfig.Certificates)
				assert.NotNil(t, tlsConfig.RootCAs)
			},
		},
		{
			name: "TLS enabled with non-existent cert file",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSCertFile = "/non/existent/cert.pem"
				cfg.TLSKeyFile = "/non/existent/key.pem"
			},
			wantErr: true,
			errMsg:  "failed to load client certificate",
		},
		{
			name: "TLS enabled with non-existent CA file",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSCAFile = "/non/existent/ca.pem"
			},
			wantErr: true,
			errMsg:  "failed to read CA certificate",
		},
		{
			name: "TLS enabled with invalid CA file",
			setup: func(cfg *Config) {
				invalidCAFile := filepath.Join(tempDir, "invalid-ca.pem")
				err := os.WriteFile(invalidCAFile, []byte("invalid certificate data"), 0644)
				require.NoError(t, err)

				cfg.TLSEnabled = true
				cfg.TLSCAFile = invalidCAFile
			},
			wantErr: true,
			errMsg:  "failed to parse CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Default()
			tt.setup(cfg)

			tlsConfig, err := cfg.CreateTLSConfig()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, tlsConfig)
			} else {
				require.NotNil(t, tlsConfig)
				if tt.validate != nil {
					tt.validate(t, tlsConfig)
				}
			}
		})
	}
}

func TestConfig_CreateAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantNil  bool
		validate func(*testing.T, gocql.Authenticator)
	}{
		{
			name:     "no credentials",
			username: "",
			password: "",
			wantNil:  true,
		},
		{
			name:     "only username",
			username: "testuser",
			password: "",
			wantNil:  true,
		},
		{
			name:     "only password",
			username: "",
			password: "testpass",
			wantNil:  true,
		},
		{
			name:     "both username and password",
			username: "testuser",
			password: "testpass",
			wantNil:  false,
			validate: func(t *testing.T, auth gocql.Authenticator) {
				passwordAuth, ok := auth.(gocql.PasswordAuthenticator)
				require.True(t, ok)
				assert.Equal(t, "testuser", passwordAuth.Username)
				assert.Equal(t, "testpass", passwordAuth.Password)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Username: tt.username,
				Password: tt.password,
			}

			auth := cfg.CreateAuthenticator()

			if tt.wantNil {
				assert.Nil(t, auth)
			} else {
				require.NotNil(t, auth)
				if tt.validate != nil {
					tt.validate(t, auth)
				}
			}
		})
	}
}

func TestConfig_CreateClusterConfig(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		setup    func(*Config)
		wantErr  bool
		errMsg   string
		validate func(*testing.T, *gocql.ClusterConfig)
	}{
		{
			name: "basic configuration",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				assert.Equal(t, []string{"127.0.0.1"}, cluster.Hosts)
				assert.Equal(t, DefaultPort, cluster.Port)
				assert.Equal(t, DefaultKeyspace, cluster.Keyspace)
				assert.Equal(t, DefaultNumConns, cluster.NumConns)
				assert.Equal(t, DefaultTimeout, cluster.Timeout)
				assert.Equal(t, DefaultConnectTimeout, cluster.ConnectTimeout)
				assert.Equal(t, gocql.Quorum, cluster.Consistency)
				assert.Equal(t, gocql.Serial, cluster.SerialConsistency)
				assert.Nil(t, cluster.Authenticator)
				assert.Nil(t, cluster.SslOpts)
			},
		},
		{
			name: "configuration with authentication",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Username = "testuser"
				cfg.Password = "testpass"
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				require.NotNil(t, cluster.Authenticator)
				passwordAuth, ok := cluster.Authenticator.(gocql.PasswordAuthenticator)
				require.True(t, ok)
				assert.Equal(t, "testuser", passwordAuth.Username)
				assert.Equal(t, "testpass", passwordAuth.Password)
			},
		},
		{
			name: "configuration with TLS",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				require.NotNil(t, cluster.SslOpts)
				require.NotNil(t, cluster.SslOpts.Config)
				assert.False(t, cluster.SslOpts.Config.InsecureSkipVerify)
				assert.Len(t, cluster.SslOpts.Config.Certificates, 1)
				assert.NotNil(t, cluster.SslOpts.Config.RootCAs)
			},
		},
		{
			name: "configuration with DC-aware routing",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.DCAwareRouting = true
				cfg.LocalDC = "datacenter1"
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				assert.NotNil(t, cluster.PoolConfig.HostSelectionPolicy)
				// We can't easily test the exact policy type, but we can verify it's set
			},
		},
		{
			name: "configuration with token-aware routing",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TokenAwareRouting = true
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				assert.NotNil(t, cluster.PoolConfig.HostSelectionPolicy)
			},
		},
		{
			name: "configuration with both DC-aware and token-aware routing",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.DCAwareRouting = true
				cfg.LocalDC = "datacenter1"
				cfg.TokenAwareRouting = true
			},
			wantErr: false,
			validate: func(t *testing.T, cluster *gocql.ClusterConfig) {
				assert.NotNil(t, cluster.PoolConfig.HostSelectionPolicy)
			},
		},
		{
			name: "configuration with invalid TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = "/non/existent/cert.pem"
				cfg.TLSKeyFile = "/non/existent/key.pem"
			},
			wantErr: true,
			errMsg:  "failed to create TLS config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			cluster, err := cfg.CreateClusterConfig()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cluster)

			if tt.validate != nil {
				tt.validate(t, cluster)
			}
		})
	}
}

func TestConfig_ValidateProduction(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid production config",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
				cfg.Username = "produser"
				cfg.Password = "prodpass"
				cfg.Consistency = "QUORUM"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			wantErr: false,
		},
		{
			name: "production config without TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
				cfg.Username = "produser"
				cfg.Password = "prodpass"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			wantErr: true,
			errMsg:  "TLS must be enabled in production",
		},
		{
			name: "production config without authentication",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = ""
				cfg.Password = ""
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			wantErr: true,
			errMsg:  "authentication credentials must be provided in production",
		},
		{
			name: "production config with insecure TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
				cfg.Username = "produser"
				cfg.Password = "prodpass"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			wantErr: true,
			errMsg:  "TLS certificate verification cannot be disabled in production",
		},
		{
			name: "production config with weak consistency",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "produser"
				cfg.Password = "prodpass"
				cfg.Consistency = "ANY"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			wantErr: true,
			errMsg:  "consistency level ANY is not recommended for production",
		},
		{
			name: "production config with insufficient hosts",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.Username = "produser"
				cfg.Password = "prodpass"
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

func TestConfig_GetConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Config)
		expected string
	}{
		{
			name: "single host without TLS",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			expected: "127.0.0.1:9042",
		},
		{
			name: "multiple hosts without TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			expected: "host1:9042,host2:9042,host3:9042",
		},
		{
			name: "single host with TLS",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
			},
			expected: "127.0.0.1:9042 (TLS)",
		},
		{
			name: "multiple hosts with TLS and multi-DC",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Hosts = []string{"host1", "host2", "host3"}
				cfg.TLSEnabled = true
				cfg.DCAwareRouting = true
				cfg.LocalDC = "datacenter1"
			},
			expected: "host1:9042,host2:9042,host3:9042 (TLS) (DC: datacenter1)",
		},
		{
			name: "custom port",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Port = 9043
				cfg.Hosts = []string{"host1", "host2"}
			},
			expected: "host1:9043,host2:9043",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			result := cfg.GetConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfig_IsMultiDC(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Config)
		expected bool
	}{
		{
			name: "not multi-DC",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			expected: false,
		},
		{
			name: "DC-aware routing without local DC",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.DCAwareRouting = true
			},
			expected: false,
		},
		{
			name: "local DC without DC-aware routing",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.LocalDC = "datacenter1"
			},
			expected: false,
		},
		{
			name: "multi-DC configuration",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.DCAwareRouting = true
				cfg.LocalDC = "datacenter1"
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			result := cfg.IsMultiDC()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfig_ApplyEnvVars_TLS(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"SCYLLADB_TLS_ENABLED",
		"SCYLLADB_TLS_CERT_FILE",
		"SCYLLADB_TLS_KEY_FILE",
		"SCYLLADB_TLS_CA_FILE",
		"SCYLLADB_USERNAME",
		"SCYLLADB_PASSWORD",
	}

	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore environment after test
	defer func() {
		for _, envVar := range envVars {
			if val, exists := originalEnv[envVar]; exists {
				os.Setenv(envVar, val)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		setup   func(*Config)
		verify  func(*testing.T, *Config)
	}{
		{
			name: "TLS environment variables",
			envVars: map[string]string{
				"SCYLLADB_TLS_ENABLED":   "true",
				"SCYLLADB_TLS_CERT_FILE": "/path/to/cert.pem",
				"SCYLLADB_TLS_KEY_FILE":  "/path/to/key.pem",
				"SCYLLADB_TLS_CA_FILE":   "/path/to/ca.pem",
			},
			setup: func(cfg *Config) {
				cfg.Default()
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TLSEnabled)
				assert.Equal(t, "/path/to/cert.pem", cfg.TLSCertFile)
				assert.Equal(t, "/path/to/key.pem", cfg.TLSKeyFile)
				assert.Equal(t, "/path/to/ca.pem", cfg.TLSCAFile)
			},
		},
		{
			name: "authentication environment variables",
			envVars: map[string]string{
				"SCYLLADB_USERNAME": "envuser",
				"SCYLLADB_PASSWORD": "envpass",
			},
			setup: func(cfg *Config) {
				cfg.Default()
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "envuser", cfg.Username)
				assert.Equal(t, "envpass", cfg.Password)
			},
		},
		{
			name: "TLS disabled via environment",
			envVars: map[string]string{
				"SCYLLADB_TLS_ENABLED": "false",
			},
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true // Set to true initially
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.False(t, cfg.TLSEnabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg := &Config{}
			tt.setup(cfg)

			err := cfg.ApplyEnvVars()
			require.NoError(t, err)

			tt.verify(t, cfg)

			// Clean up environment variables
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestConfig_ValidateTLSCertificates(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "TLS disabled",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = false
			},
			wantErr: false,
		},
		{
			name: "TLS enabled without certificates",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
			},
			wantErr: false,
		},
		{
			name: "valid certificates",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
			},
			wantErr: false,
		},
		{
			name: "non-existent cert file",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSCertFile = "/non/existent/cert.pem"
				cfg.TLSKeyFile = "/non/existent/key.pem"
			},
			wantErr: true,
			errMsg:  "TLS cert file does not exist",
		},
		{
			name: "non-existent key file",
			setup: func(cfg *Config) {
				certFile, _, _ := createTestCertificates(t, tempDir)
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = "/non/existent/key.pem"
			},
			wantErr: true,
			errMsg:  "TLS key file does not exist",
		},
		{
			name: "invalid certificate pair",
			setup: func(cfg *Config) {
				certFile, keyFile, _ := createTestCertificates(t, tempDir)
				// Create another certificate pair
				certFile2, _, _ := createTestCertificates(t, tempDir)

				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile2 // Use cert from different pair
				cfg.TLSKeyFile = keyFile    // Use key from original pair
			},
			wantErr: true,
			errMsg:  "invalid TLS certificate/key pair",
		},
		{
			name: "non-existent CA file",
			setup: func(cfg *Config) {
				cfg.TLSEnabled = true
				cfg.TLSCAFile = "/non/existent/ca.pem"
			},
			wantErr: true,
			errMsg:  "TLS CA file does not exist",
		},
		{
			name: "invalid CA file",
			setup: func(cfg *Config) {
				invalidCAFile := filepath.Join(tempDir, "invalid-ca.pem")
				err := os.WriteFile(invalidCAFile, []byte("invalid certificate data"), 0644)
				require.NoError(t, err)

				cfg.TLSEnabled = true
				cfg.TLSCAFile = invalidCAFile
			},
			wantErr: true,
			errMsg:  "invalid TLS CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Default()
			tt.setup(cfg)

			err := cfg.ValidateTLSCertificates()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetSecurityLevel(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		setup         func(*Config)
		expectedLevel string
		minScore      int
		maxScore      int
		checkIssues   func(*testing.T, []string)
	}{
		{
			name: "excellent security",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "QUORUM"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			expectedLevel: "Excellent",
			minScore:      90,
			maxScore:      100,
			checkIssues: func(t *testing.T, issues []string) {
				assert.Empty(t, issues)
			},
		},
		{
			name: "good security",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = false
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "QUORUM"
				cfg.Hosts = []string{"host1", "host2", "host3"}
			},
			expectedLevel: "Good",
			minScore:      70,
			maxScore:      89,
			checkIssues: func(t *testing.T, issues []string) {
				assert.Contains(t, issues, "Client certificate authentication not configured")
			},
		},
		{
			name: "fair security",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "ONE"
				cfg.Hosts = []string{"host1"}
			},
			expectedLevel: "Fair",
			minScore:      50,
			maxScore:      69,
			checkIssues: func(t *testing.T, issues []string) {
				assert.Contains(t, issues, "TLS certificate verification is disabled")
				assert.Contains(t, issues, "Weak consistency level: ONE")
				assert.Contains(t, issues, "Insufficient hosts for high availability")
			},
		},
		{
			name: "poor security",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
				cfg.Username = "user"
				cfg.Password = "pass"
				cfg.Consistency = "ANY"
			},
			expectedLevel: "Poor",
			minScore:      30,
			maxScore:      49,
			checkIssues: func(t *testing.T, issues []string) {
				assert.Contains(t, issues, "TLS encryption is disabled")
				assert.Contains(t, issues, "Weak consistency level: ANY")
			},
		},
		{
			name: "critical security",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
				cfg.Username = ""
				cfg.Password = ""
				cfg.Consistency = "ANY"
			},
			expectedLevel: "Critical",
			minScore:      0,
			maxScore:      29,
			checkIssues: func(t *testing.T, issues []string) {
				assert.Contains(t, issues, "TLS encryption is disabled")
				assert.Contains(t, issues, "No authentication credentials configured")
				assert.Contains(t, issues, "Weak consistency level: ANY")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			level, score, issues := cfg.GetSecurityLevel()

			assert.Equal(t, tt.expectedLevel, level)
			assert.GreaterOrEqual(t, score, tt.minScore)
			assert.LessOrEqual(t, score, tt.maxScore)

			if tt.checkIssues != nil {
				tt.checkIssues(t, issues)
			}
		})
	}
}

func TestConfig_CreateSecureClusterConfig(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid secure configuration",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
				cfg.Username = "user"
				cfg.Password = "pass"
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
			errMsg:  "TLS must be enabled for secure connections",
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
			errMsg:  "authentication credentials are required for secure connections",
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
			errMsg:  "TLS certificate verification cannot be disabled for secure connections",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			cluster, err := cfg.CreateSecureClusterConfig()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, cluster)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cluster)

				// Verify secure settings
				assert.False(t, cluster.DisableInitialHostLookup)
				assert.False(t, cluster.IgnorePeerAddr)
				assert.NotNil(t, cluster.SslOpts)
				assert.NotNil(t, cluster.Authenticator)
			}
		})
	}
}

func TestConfig_GetTLSInfo(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name   string
		setup  func(*Config)
		verify func(*testing.T, map[string]interface{})
	}{
		{
			name: "TLS disabled",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = false
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.False(t, info["enabled"].(bool))
				assert.False(t, info["client_cert_configured"].(bool))
				assert.False(t, info["ca_configured"].(bool))
			},
		},
		{
			name: "TLS enabled without certificates",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSInsecureSkipVerify = true
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.True(t, info["enabled"].(bool))
				assert.True(t, info["insecure_skip_verify"].(bool))
				assert.False(t, info["client_cert_configured"].(bool))
				assert.False(t, info["ca_configured"].(bool))
			},
		},
		{
			name: "TLS with certificates",
			setup: func(cfg *Config) {
				certFile, keyFile, caFile := createTestCertificates(t, tempDir)
				cfg.Default()
				cfg.TLSEnabled = true
				cfg.TLSCertFile = certFile
				cfg.TLSKeyFile = keyFile
				cfg.TLSCAFile = caFile
				cfg.TLSInsecureSkipVerify = false
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.True(t, info["enabled"].(bool))
				assert.False(t, info["insecure_skip_verify"].(bool))
				assert.True(t, info["client_cert_configured"].(bool))
				assert.True(t, info["ca_configured"].(bool))

				// Check certificate details
				assert.Contains(t, info, "cert_subject")
				assert.Contains(t, info, "cert_issuer")
				assert.Contains(t, info, "cert_not_before")
				assert.Contains(t, info, "cert_not_after")
				assert.Contains(t, info, "cert_expired")
				assert.Contains(t, info, "cert_expires_soon")

				assert.False(t, info["cert_expired"].(bool))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			info := cfg.GetTLSInfo()
			tt.verify(t, info)
		})
	}
}

func TestConfig_GetAuthInfo(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Config)
		verify func(*testing.T, map[string]interface{})
	}{
		{
			name: "no authentication",
			setup: func(cfg *Config) {
				cfg.Default()
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.False(t, info["username_configured"].(bool))
				assert.False(t, info["password_configured"].(bool))
				assert.False(t, info["auth_enabled"].(bool))
				_, hasUsername := info["username"]
				assert.False(t, hasUsername)
			},
		},
		{
			name: "only username",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Username = "testuser"
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.True(t, info["username_configured"].(bool))
				assert.False(t, info["password_configured"].(bool))
				assert.False(t, info["auth_enabled"].(bool))
				assert.Equal(t, "testuser", info["username"].(string))
			},
		},
		{
			name: "only password",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Password = "testpass"
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.False(t, info["username_configured"].(bool))
				assert.True(t, info["password_configured"].(bool))
				assert.False(t, info["auth_enabled"].(bool))
			},
		},
		{
			name: "full authentication",
			setup: func(cfg *Config) {
				cfg.Default()
				cfg.Username = "testuser"
				cfg.Password = "testpass"
			},
			verify: func(t *testing.T, info map[string]interface{}) {
				assert.True(t, info["username_configured"].(bool))
				assert.True(t, info["password_configured"].(bool))
				assert.True(t, info["auth_enabled"].(bool))
				assert.Equal(t, "testuser", info["username"].(string))

				// Verify password is never included
				_, hasPassword := info["password"]
				assert.False(t, hasPassword)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.setup(cfg)

			info := cfg.GetAuthInfo()
			tt.verify(t, info)
		})
	}
}
