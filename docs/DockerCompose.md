🔧 Key Features of the Build System:
Multi-Stage Build Process:
Builder stage: Compiles Go binaries with ScyllaDB support
Runtime stage: Minimal Alpine image with just the essentials
Configuration Management:
Environment variables for all ScyllaDB settings
Configuration templates for different deployment scenarios
Automatic configuration override from environment
Security & Performance:
Non-root user execution
Static binaries with no external dependencies
Health checks built into the image
Minimal attack surface with Alpine base
Development Features:
Live configuration updates via environment variables
Debug builds with additional tooling
Multi-architecture support (AMD64, ARM64)
CI/CD integration examples
📋 Environment Variables Supported:
# ScyllaDB Configuration
SCYLLADB_HOSTS=scylla1,scylla2,scylla3
SCYLLADB_PORT=9042
SCYLLADB_KEYSPACE=ipfs_pins
SCYLLADB_USERNAME=cluster_user
SCYLLADB_PASSWORD=secure_password
SCYLLADB_CONSISTENCY=QUORUM

# Cluster Configuration  
CLUSTER_PEERNAME=my-node
CLUSTER_SECRET=shared_secret
CLUSTER_BOOTSTRAP_PEERS=peer1,peer2,peer3
CLUSTER_LOGLEVEL=info

# API Configuration
CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS=/ip4/0.0.0.0/tcp/9094
CLUSTER_IPFSPROXY_LISTENMULTIADDRESS=/ip4/0.0.0.0/tcp/9095
🎯 Usage Examples:
Basic Run:
docker run -d \
  --name ipfs-cluster-scylladb \
  -p 9094:9094 -p 9095:9095 -p 9096:9096 -p 8888:8888 \
  -e SCYLLADB_HOSTS=scylla-host \
  -e SCYLLADB_KEYSPACE=ipfs_pins \
  ipfs-cluster-scylladb:latest
Production Setup:
docker run -d \
  --name cluster-prod \
  -e SCYLLADB_HOSTS=scylla1,scylla2,scylla3 \
  -e SCYLLADB_CONSISTENCY=QUORUM \
  -e CLUSTER_CONFIG_TEMPLATE=enterprise/cluster.json \
  -v cluster_data:/data/ipfs-cluster \
  ipfs-cluster-scylladb:latest
Now you have a complete Docker build system that allows you to:

Build custom images with ScyllaDB integration
Configure everything via environment variables
Deploy in any environment (development, testing, production)
Use pre-built configuration templates
Monitor and debug the running containers
The build system is cross-platform compatible and includes comprehensive documentation for all use cases!