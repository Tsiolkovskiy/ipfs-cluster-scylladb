package scyllastate

import (
	"fmt"
	"log"
)

// ExampleTLSConfiguration demonstrates how to configure TLS for ScyllaDB
func ExampleTLSConfiguration() {
	// Create a new configuration
	cfg := &Config{}
	cfg.Default()

	// Configure TLS settings
	cfg.TLSEnabled = true
	cfg.TLSCertFile = "/path/to/client.crt"
	cfg.TLSKeyFile = "/path/to/client.key"
	cfg.TLSCAFile = "/path/to/ca.crt"
	cfg.TLSInsecureSkipVerify = false

	// Configure authentication
	cfg.Username = "scylla_user"
	cfg.Password = "secure_password"

	// Configure hosts for production
	cfg.Hosts = []string{
		"scylla-node-1.example.com",
		"scylla-node-2.example.com",
		"scylla-node-3.example.com",
	}

	// Set strong consistency for production
	cfg.Consistency = "QUORUM"
	cfg.SerialConsistency = "SERIAL"

	// Validate the configuration
	if err := cfg.ValidateProduction(); err != nil {
		log.Fatalf("Production validation failed: %v", err)
	}

	// Get security assessment
	level, score, issues := cfg.GetSecurityLevel()
	fmt.Printf("Security Level: %s (Score: %d/100)\n", level, score)
	if len(issues) > 0 {
		fmt.Println("Security Issues:")
		for _, issue := range issues {
			fmt.Printf("  - %s\n", issue)
		}
	}

	// Create a secure cluster configuration
	cluster, err := cfg.CreateSecureClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create secure cluster config: %v", err)
	}

	fmt.Printf("Cluster configured with %d hosts\n", len(cluster.Hosts))
	fmt.Printf("TLS enabled: %t\n", cluster.SslOpts != nil)
	fmt.Printf("Authentication enabled: %t\n", cluster.Authenticator != nil)
}

// ExampleDevelopmentConfiguration demonstrates a development configuration
func ExampleDevelopmentConfiguration() {
	cfg := &Config{}
	cfg.Default()

	// Development settings - less secure but easier to set up
	cfg.TLSEnabled = true
	cfg.TLSInsecureSkipVerify = true // Only for development!
	cfg.Username = "dev_user"
	cfg.Password = "dev_password"
	cfg.Consistency = "ONE" // Faster for development

	// Validate with development options
	opts := DevelopmentValidationOptions()
	if err := cfg.ValidateWithOptions(opts); err != nil {
		log.Fatalf("Development validation failed: %v", err)
	}

	// Get TLS and auth info
	tlsInfo := cfg.GetTLSInfo()
	authInfo := cfg.GetAuthInfo()

	fmt.Printf("TLS enabled: %t\n", tlsInfo["enabled"])
	fmt.Printf("TLS insecure: %t\n", tlsInfo["insecure_skip_verify"])
	fmt.Printf("Auth enabled: %t\n", authInfo["auth_enabled"])
}

// ExampleMultiDatacenterConfiguration demonstrates multi-DC setup
func ExampleMultiDatacenterConfiguration() {
	cfg := &Config{}
	cfg.Default()

	// Multi-DC configuration
	cfg.Hosts = []string{
		"dc1-scylla-1.example.com",
		"dc1-scylla-2.example.com",
		"dc2-scylla-1.example.com",
		"dc2-scylla-2.example.com",
	}

	// Enable DC-aware routing
	cfg.DCAwareRouting = true
	cfg.LocalDC = "datacenter1"
	cfg.TokenAwareRouting = true

	// Use LOCAL_QUORUM for multi-DC
	cfg.Consistency = "LOCAL_QUORUM"

	// Security settings
	cfg.TLSEnabled = true
	cfg.TLSCertFile = "/etc/ssl/certs/scylla-client.crt"
	cfg.TLSKeyFile = "/etc/ssl/private/scylla-client.key"
	cfg.TLSCAFile = "/etc/ssl/certs/scylla-ca.crt"
	cfg.Username = "cluster_service"
	cfg.Password = "production_password"

	fmt.Printf("Multi-DC setup: %t\n", cfg.IsMultiDC())
	fmt.Printf("Connection string: %s\n", cfg.GetConnectionString())

	// Create cluster configuration
	cluster, err := cfg.CreateClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create cluster config: %v", err)
	}

	fmt.Printf("Host selection policy configured: %t\n", cluster.PoolConfig.HostSelectionPolicy != nil)
}

// ExampleEnvironmentConfiguration demonstrates environment variable usage
func ExampleEnvironmentConfiguration() {
	cfg := &Config{}
	cfg.Default()

	// Apply environment variables
	// These would typically be set in the environment:
	// export SCYLLADB_HOSTS="scylla1.prod.com,scylla2.prod.com,scylla3.prod.com"
	// export SCYLLADB_TLS_ENABLED="true"
	// export SCYLLADB_TLS_CERT_FILE="/etc/ssl/certs/client.crt"
	// export SCYLLADB_TLS_KEY_FILE="/etc/ssl/private/client.key"
	// export SCYLLADB_TLS_CA_FILE="/etc/ssl/certs/ca.crt"
	// export SCYLLADB_USERNAME="prod_user"
	// export SCYLLADB_PASSWORD="prod_password"
	// export SCYLLADB_LOCAL_DC="us-east-1"

	if err := cfg.ApplyEnvVars(); err != nil {
		log.Fatalf("Failed to apply environment variables: %v", err)
	}

	// Get effective configuration
	effectiveCfg, err := cfg.GetEffectiveConfig()
	if err != nil {
		log.Fatalf("Failed to get effective config: %v", err)
	}

	fmt.Printf("Effective hosts: %v\n", effectiveCfg.Hosts)
	fmt.Printf("TLS enabled: %t\n", effectiveCfg.TLSEnabled)
}
