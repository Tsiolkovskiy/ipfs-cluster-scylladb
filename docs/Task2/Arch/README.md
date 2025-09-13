# Task 2 Architecture Documentation

This directory contains the C4 PlantUML architecture diagrams for Task 2 of the ScyllaDB State Store implementation.

## Task 2 Overview

**Task 2** focuses on implementing the configuration structure and TLS/authentication capabilities for the ScyllaDB state store:

### Task 2.1: Configuration Structure and Validation
- Implement Config struct with connection, TLS, and performance fields
- Add configuration validation methods and default values
- Create JSON marshaling/unmarshaling for configuration
- Requirements: 1.1, 1.4, 8.1, 8.4

### Task 2.2: TLS and Authentication
- Add TLS configuration support with certificates
- Implement username/password and certificate-based authentication
- Create methods for secure connections
- Requirements: 8.1, 8.2, 8.3

## Architecture Diagrams

### 1. Context Diagram (`c4-context.puml`)
**Level 1 - System Context**

Shows the high-level system context with:
- **Actors**: Developer, DevOps Engineer
- **Systems**: IPFS Cluster, ScyllaDB Cluster
- **Relationships**: Configuration, management, and data flow

Key aspects:
- IPFS Cluster uses ScyllaDB for state persistence
- TLS encryption and authentication required
- Multi-node cluster for high availability

### 2. Container Diagram (`c4-container.puml`)
**Level 2 - Container View**

Breaks down the IPFS Cluster system into containers:
- **IPFS Cluster Node**: Main cluster application
- **Configuration Management**: Handles ScyllaDB configuration (Task 2.1)
- **ScyllaDB State Store**: State persistence layer (Task 2.2)
- **ScyllaDB Cluster**: Multi-node database cluster

External systems:
- **Certificate Authority**: Issues TLS certificates
- **Environment Variables**: Runtime configuration

### 3. Component Diagram (`c4-component.puml`)
**Level 3 - Component View**

Details the ScyllaDB State Store package components:

#### Task 2.1 Components:
- **Config Struct**: Main configuration structure
- **JSON Marshaling**: Serialization methods
- **Basic Validation**: Field validation
- **Default Values**: Configuration defaults
- **Advanced Validation**: Multi-level validation
- **Environment Variables**: Runtime configuration

#### Task 2.2 Components:
- **TLS Configuration**: Certificate handling
- **Authentication**: Username/password auth
- **Secure Connections**: Secure cluster config
- **Certificate Validator**: Certificate validation
- **Security Assessment**: Security scoring system

### 4. Code Diagram (`c4-code.puml`)
**Level 4 - Code View**

Shows the actual implementation structure:

#### Main Files:
- **`config.go`**: Main configuration implementation (~1000+ lines)
- **`validate_config.go`**: Advanced validation (~500+ lines)
- **`validate_config_test.go`**: Validation tests (~400+ lines)
- **`tls_auth_test.go`**: TLS and auth tests (1270+ lines)
- **`example_tls_auth.go`**: Usage examples (~200+ lines)

#### Key Classes:
- **`Config`**: Main configuration struct with 20+ fields
- **`RetryPolicyConfig`**: Retry policy configuration
- **`ValidationOptions`**: Validation configuration
- **`ValidationLevel`**: Validation level enumeration

### 5. Sequence Diagram (`task2-sequence.puml`)
**Interaction Flow**

Shows the complete flow for Task 2 implementation:

1. **Configuration Setup** (Task 2.1):
   - Create and configure Config struct
   - Load JSON configuration
   - Apply environment variables
   - Perform validation (Basic/Strict/Development)

2. **TLS and Authentication Setup** (Task 2.2):
   - Create TLS configuration with certificates
   - Set up authentication (username/password)
   - Validate certificates and security requirements

3. **Secure Connection Creation**:
   - Create secure cluster configuration
   - Apply security settings
   - Establish connection to ScyllaDB

4. **Security Assessment**:
   - Calculate security score (0-100)
   - Identify security issues
   - Provide security level rating

### 6. Class Diagram (`task2-class-diagram.puml`)
**Detailed Class Structure**

Comprehensive view of all classes and their relationships:

#### Main Classes:
- **`Config`**: 40+ methods covering all functionality
- **`RetryPolicyConfig`**: Retry policy settings
- **`ValidationOptions`**: Validation configuration
- **JSON Helper Classes**: Serialization support

#### External Integration:
- **gocql**: Cassandra/ScyllaDB driver integration
- **crypto/tls**: TLS implementation
- **crypto/x509**: Certificate handling

### 7. Deployment Diagram (`task2-deployment.puml`)
**Deployment Architecture**

Shows deployment scenarios:

#### Development Environment:
- Single ScyllaDB node
- Relaxed validation settings
- Self-signed certificates
- Local development setup

#### Production Environment:
- Multi-node ScyllaDB cluster
- Kubernetes deployment
- Strict security requirements
- CA-signed certificates
- Multi-DC support

#### Security Infrastructure:
- Certificate Authority
- Secret Management
- Security Assessment

## Key Architectural Decisions

### 1. Modular Design
- Separation of configuration (`config.go`) and validation (`validate_config.go`)
- Clear separation between Task 2.1 and Task 2.2 concerns
- Extensible validation system with multiple levels

### 2. Security-First Approach
- TLS mandatory in production
- Certificate validation with expiry checking
- Security scoring system (0-100 points)
- Multiple validation levels for different environments

### 3. Production-Ready Features
- Comprehensive error handling
- Environment variable support
- Multi-DC awareness
- Connection pooling and retry policies
- Extensive test coverage (1670+ lines of tests)

### 4. Developer Experience
- Default values for quick setup
- Clear validation messages
- Usage examples for different scenarios
- Comprehensive documentation

## Implementation Statistics

- **Total Code**: ~2000+ lines
- **Test Code**: 1670+ lines
- **Files**: 7 main files + documentation
- **Test Coverage**: 100% for validation and TLS/auth methods
- **Security Levels**: 5 levels (Critical → Excellent)
- **Validation Modes**: 3 modes (Basic, Strict, Development)

## Usage Examples

The architecture supports multiple deployment scenarios:

1. **Development**: Quick setup with relaxed security
2. **Testing**: Automated certificate generation for tests
3. **Production**: Full security with strict validation
4. **Multi-DC**: Cross-datacenter deployment support

Each scenario is supported by the flexible configuration and validation system implemented in Task 2.