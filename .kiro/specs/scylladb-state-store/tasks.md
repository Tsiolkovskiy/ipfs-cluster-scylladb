# Implementation Plan

- [x] 1. Setup project infrastructure and basic interfaces
  - Create directory structure for new ScyllaDB state store module
  - Define basic interfaces and data types for integration
  - Configure Go module dependencies (gocql, testify, prometheus)
  - _Requirements: 1.1, 4.1_

- [x] 2. Implement ScyllaDB configuration
- [x] 2.1 Create configuration structure and validation
  - Implement Config structure with connection, TLS, performance fields
  - Add configuration validation methods and default values
  - Create JSON marshaling/unmarshaling for configuration
  - _Requirements: 1.1, 1.4, 8.1, 8.4_

- [x] 2.2 Implement TLS and authentication
  - Add TLS configuration support with certificates
  - Implement username/password and certificate-based authentication
  - Create methods for establishing secure connections
  - _Requirements: 8.1, 8.2, 8.3_

- [x] 3. Create database schema and migrations
- [x] 3.1 Define ScyllaDB table schema
  - Create CQL scripts for keyspace and tables pins_by_cid, placements_by_cid, pins_by_peer
  - Implement optimized partitioning by mh_prefix
  - Add compaction and TTL settings for optimal performance
  - _Requirements: 2.1, 2.2, 7.3_

- [x] 3.2 Implement migration system
  - Create database schema versioning mechanism
  - Implement automatic migration application at startup
  - Add schema version compatibility checks
  - _Requirements: 4.3_

- [x] 4. Main ScyllaState implementation
- [x] 4.1 Create basic ScyllaState structure
  - Implement ScyllaState structure with gocql.Session and configuration
  - Add initialization and ScyllaDB cluster connection methods
  - Create prepared statements for main operations
  - _Requirements: 4.1, 4.2, 7.1_

- [x] 4.2 Implement state.ReadOnly interface methods
  - Implement Get() method for retrieving pin by CID
  - Add Has() method for checking pin existence
  - Create List() method with streaming support for large results
  - _Requirements: 2.2, 4.2_

- [x] 4.3 Implement state.WriteOnly interface methods
  - Create Add() method for adding/updating pins
  - Implement Rm() method for pin removal with proper cleanup
  - Add conditional update support to prevent race conditions
  - _Requirements: 2.1, 2.3, 2.4_

- [x] 5. Implement batch processing and performance
- [x] 5.1 Create ScyllaBatchingState
  - Implement BatchingState structure with gocql.Batch support
  - Add Commit() method for applying batch operations
  - Create logic for grouping operations into efficient batches
  - _Requirements: 4.1, 7.2_

- [x] 5.2 Optimize connection performance
  - Configure connection pools with optimal parameters
  - Implement token-aware and DC-aware routing for multi-DC
  - Add prepared statement caching
  - _Requirements: 6.1, 6.4, 7.1_

- [x] 6. Error handling and fault tolerance
- [x] 6.1 Implement retry policies
  - Create RetryPolicy with exponential backoff
  - Add logic for determining retryable errors
  - Implement retries with context awareness and cancellation support
  - _Requirements: 3.1, 3.2_

- [x] 6.2 Add graceful degradation
  - Implement ScyllaDB node unavailability handling
  - Add automatic failover to available replicas
  - Create recovery mechanisms after network partitions
  - _Requirements: 3.1, 3.3, 3.4_

- [ ] 7. Monitoring and observability
- [x] 7.1 Implement Prometheus metrics
  - Create metrics for operation latency, throughput, error rates
  - Add connection and pool state metrics
  - Implement ScyllaDB-specific metrics (timeouts, unavailability errors)
  - _Requirements: 5.1, 5.3_

- [x] 7.2 Add structured logging
  - Implement contextual operation logging with CID and duration
  - Add detailed error logging with ScyllaDB error codes
  - Create debug-level tracing for troubleshooting
  - _Requirements: 5.2, 5.4_

- [x] 8. Serialization and compatibility
- [x] 8.1 Implement pin serialization
  - Use existing api.Pin.ProtoMarshal() for compatibility
  - Add serializePin/deserializePin methods
  - Create versioning handling for serialized data
  - _Requirements: 4.2_

- [x] 8.2 Add Marshal/Unmarshal support
  - Implement Marshal() method for state export
  - Create Unmarshal() method for state import
  - Add Migrate() method for updating old formats
  - _Requirements: 4.3_

- [x] 9. Data migration utilities
- [x] 9.1 Create migration tools
  - Implement Migrator for transferring data from existing backends
  - Add batch migration support for large data volumes
  - Create data integrity validation after migration
  - _Requirements: 4.3_

- [x] 9.2 Add command-line utilities
  - Create commands for state export/import
  - Implement utilities for ScyllaDB state checking
  - Add tools for monitoring migration process
  - _Requirements: 4.3_

- [-] 10. Comprehensive testing
- [x] 10.1 Create unit tests
  - Write tests for all ScyllaState methods with mock ScyllaDB
  - Add configuration and validation tests
  - Create retry policy and error handling tests
  - _Requirements: 4.4_

- [x] 10.2 Implement integration tests
  - Create tests with real ScyllaDB in Docker containers
  - Add multi-node cluster and failover tests
  - Implement data migration tests between backends
  - _Requirements: 4.4, 3.1, 3.3_

- [x] 10.3 Add performance tests
  - Create benchmarks for Add/Get/List/Rm operations
  - Implement load tests with large data volumes
  - Add batch operation performance tests
  - _Requirements: 7.1, 7.2_

- [ ] 11. Documentation and examples
- [x] 11.1 Create configuration documentation
  - Write ScyllaDB state store setup guide
  - Add configuration examples for various scenarios
  - Create multi-datacenter deployment documentation
  - _Requirements: 1.1, 6.1, 6.2_

- [x] 11.2 Add usage examples
  - Create code examples for initialization and usage
  - Implement migration examples from existing backends
  - Add monitoring and troubleshooting examples
  - _Requirements: 4.1, 5.1_

- [x] 12. Implement Security/AuthZ layer
- [x] 12.1 Create Auth Service component
  - Implement AuthService structure with JWT, API keys, DID signatures support
  - Add Authenticator and Authorizer interfaces
  - Create Audit Logger for recording all access attempts
  - _Requirements: 9.1, 9.4_

- [x] 12.2 Implement authentication
  - Add JWT token support with signature and expiration validation
  - Implement API key authentication with hashing and rotation
  - Create DID/Web3 signature support for decentralized authentication
  - _Requirements: 9.1, 9.2_

- [x] 12.3 Implement authorization
  - Create Policy Engine for RBAC/ABAC checks
  - Implement access control checks at operation level (pin/unpin/list)
  - Add tenant-based data isolation support
  - _Requirements: 9.3, 9.5_

- [x] 12.4 Integrate with Identity Providers
  - Add OIDC integration support with Keycloak/Auth0
  - Implement Web3 wallet integration (MetaMask, WalletConnect)
  - Create fallback to built-in user system
  - _Requirements: 9.2_

- [-] 13. Implement billing system
- [x] 13.1 Create Billing Service component
  - Implement BillingService structure with Usage Collector, Cost Calculator, Invoice Emitter components
  - Add storage_usage and billing_events tables to ScyllaDB
  - Create NATS billing.event topic for asynchronous processing
  - _Requirements: 10.1, 10.2_

- [x] 13.2 Implement usage metrics collection
  - Create Usage Collector for storage usage data aggregation
  - Implement daily rollup operations by owner_id
  - Add pin size and storage time tracking
  - _Requirements: 10.1, 10.2_

- [x] 13.3 Implement cost calculation
  - Create Cost Calculator with configurable pricing tiers
  - Implement various pricing models (per GB/month, per operation)
  - Add discount and promo code support
  - _Requirements: 10.3_

- [x] 13.4 Integrate with payment systems
  - Implement Stripe integration for traditional payments
  - Add Filecoin Payment Channels support
  - Create L2 solution integration (Polygon, Arbitrum)
  - _Requirements: 10.4_

- [x] 14. Add IPFS-cluster integration examples
  - Create example applications demonstrating optimized configuration
  - Add docker-compose files for quick test environment deployment
  - Write scripts for automatic performance benchmarking
  - Create dashboard for monitoring metrics in Grafana
  - After task 14, how do we build "docker image" on docker compose
  
- [ ] 14. Implement enterprise monitoring
- [ ] 14.1 Extend Prometheus metrics
  - Add SLO metrics (pin_end_to_end_seconds, global_drift_ratio)
  - Implement metrics for all components (Auth, Billing, Workers)
  - Create recording rules for metric aggregation
  - _Requirements: 11.1, 11.2_

- [ ] 14.2 Create alerting system
  - Implement alerts for SLO violations (latency > 60s, drift > 0.5%)
  - Add alerts for critical component errors
  - Create runbook for common issue resolution
  - _Requirements: 11.3_

- [ ] 14.3 Create Grafana dashboards
  - Implement dashboard with key SLO metrics
  - Add component monitoring dashboards (ScyllaDB, NATS, Workers)
  - Create billing dashboard with usage and revenue metrics
  - _Requirements: 11.4_

- [ ] 15. Implement enterprise architecture
- [ ] 15.1 Create API Gateway component
  - Implement API Gateway with Auth Service integration
  - Add rate limiting and request validation
  - Create middleware for logging and metrics
  - _Requirements: 12.1, 12.5_

- [ ] 15.2 Implement NATS integration
  - Configure NATS JetStream for reliable message delivery
  - Create topics pin.request, pin.assign, pin.status, billing.event
  - Implement durable consumers with retry logic
  - _Requirements: 12.2_

- [ ] 15.3 Create Worker Agent component
  - Implement stateless Pin Worker Agent with NATS integration
  - Add Kubo HTTP Client with retry and timeout logic
  - Create Status Reporter for sending operation results
  - _Requirements: 12.1, 12.3_

- [ ] 15.4 Implement Reconciler Engine
  - Create control loop for comparing desired vs actual state
  - Implement task scheduler for load balancing
  - Add TTL Scheduler for automatic expired pin removal
  - _Requirements: 12.4_

- [ ] 16. Enterprise testing and deployment
- [ ] 16.1 Create enterprise component integration tests
  - Write Auth Service tests with various authentication methods
  - Create Billing Service tests with mock payment systems
  - Implement end-to-end tests for complete pinning pipeline
  - _Requirements: 9.1, 10.1, 12.1_

- [ ] 16.2 Create load tests
  - Implement performance tests for enterprise workloads
  - Add scaling tests with multiple workers
  - Create fault tolerance tests with component failure simulation
  - _Requirements: 12.1, 12.4_

- [ ] 16.3 Prepare Docker and Kubernetes deployment
  - Create Dockerfile for all components
  - Implement Kubernetes manifests with ConfigMaps and Secrets
  - Add Helm charts for simplified deployment
  - _Requirements: 12.5_

- [ ] 17. Integration into IPFS-Cluster
- [ ] 17.1 Add support to cluster configuration
  - Modify cluster configuration to support ScyllaDB backend
  - Add factory methods for creating ScyllaState
  - Integrate into existing configuration system
  - _Requirements: 1.1, 1.2_

- [ ] 17.2 Update IPFS-Cluster documentation
  - Add ScyllaDB to state backend documentation
  - Create migration guide for existing users
  - Update configuration examples in repository
  - _Requirements: 4.3_

- [ ] 17.3 Create enterprise documentation
  - Write enterprise architecture deployment guide
  - Create Security/AuthZ and billing setup documentation
  - Add external system integration examples
  - _Requirements: 9.2, 10.4, 11.5_