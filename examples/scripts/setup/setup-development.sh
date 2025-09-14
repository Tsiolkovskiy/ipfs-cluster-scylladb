#!/bin/bash

# IPFS-Cluster ScyllaDB Integration - Development Environment Setup Script
# This script sets up a complete development environment for testing ScyllaDB integration

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if Docker is installed and running
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! docker info &> /dev/null; then
        error "Docker is not running. Please start Docker."
        exit 1
    fi
    
    # Check if docker-compose is available
    if ! command -v docker-compose &> /dev/null; then
        error "docker-compose is not installed. Please install docker-compose."
        exit 1
    fi
    
    # Check available disk space (minimum 10GB)
    available_space=$(df / | awk 'NR==2 {print $4}')
    if [ "$available_space" -lt 10485760 ]; then  # 10GB in KB
        warning "Less than 10GB of disk space available. The setup might fail."
    fi
    
    success "Prerequisites check passed"
}

# Setup development environment
setup_development_environment() {
    log "Setting up development environment..."
    
    cd "${PROJECT_ROOT}/examples/docker-compose/development"
    
    # Stop any existing containers
    log "Stopping existing containers..."
    docker-compose down -v 2>/dev/null || true
    
    # Pull latest images
    log "Pulling latest Docker images..."
    docker-compose pull
    
    # Start the environment
    log "Starting development environment..."
    docker-compose up -d
    
    # Wait for services to be ready
    log "Waiting for services to start..."
    
    # Wait for ScyllaDB
    log "Waiting for ScyllaDB to be ready..."
    for i in {1..60}; do
        if docker exec scylla-dev cqlsh -e "SELECT now() FROM system.local;" &>/dev/null; then
            success "ScyllaDB is ready"
            break
        fi
        if [ $i -eq 60 ]; then
            error "ScyllaDB failed to start within timeout"
            return 1
        fi
        sleep 5
        echo -n "."
    done
    
    # Wait for IPFS
    log "Waiting for IPFS to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:5001/api/v0/version &>/dev/null; then
            success "IPFS is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "IPFS failed to start within timeout"
            return 1
        fi
        sleep 3
        echo -n "."
    done
    
    # Wait for IPFS-Cluster
    log "Waiting for IPFS-Cluster to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:9094/api/v0/id &>/dev/null; then
            success "IPFS-Cluster is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "IPFS-Cluster failed to start within timeout"
            return 1
        fi
        sleep 3
        echo -n "."
    done
    
    # Wait for Prometheus
    log "Waiting for Prometheus to be ready..."
    for i in {1..20}; do
        if curl -s http://localhost:9090/-/ready &>/dev/null; then
            success "Prometheus is ready"
            break
        fi
        if [ $i -eq 20 ]; then
            warning "Prometheus may not be ready yet"
            break
        fi
        sleep 3
        echo -n "."
    done
    
    # Wait for Grafana
    log "Waiting for Grafana to be ready..."
    for i in {1..20}; do
        if curl -s http://localhost:3000/api/health &>/dev/null; then
            success "Grafana is ready"
            break
        fi
        if [ $i -eq 20 ]; then
            warning "Grafana may not be ready yet"
            break
        fi
        sleep 3
        echo -n "."
    done
    
    success "Development environment is ready!"
}

# Verify setup
verify_setup() {
    log "Verifying setup..."
    
    # Test ScyllaDB connection
    log "Testing ScyllaDB connection..."
    if docker exec scylla-dev cqlsh -e "DESCRIBE KEYSPACE ipfs_pins;" &>/dev/null; then
        success "ScyllaDB keyspace created successfully"
    else
        error "ScyllaDB keyspace not found"
        return 1
    fi
    
    # Test IPFS-Cluster API
    log "Testing IPFS-Cluster API..."
    cluster_id=$(curl -s http://localhost:9094/api/v0/id | jq -r '.id' 2>/dev/null)
    if [ "$cluster_id" != "null" ] && [ -n "$cluster_id" ]; then
        success "IPFS-Cluster API is working (ID: $cluster_id)"
    else
        error "IPFS-Cluster API is not responding correctly"
        return 1
    fi
    
    # Test pin operation
    log "Testing pin operation..."
    test_cid="QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
    if curl -s -X POST "http://localhost:9094/api/v0/pins/$test_cid" &>/dev/null; then
        success "Pin operation test successful"
        
        # Clean up test pin
        curl -s -X DELETE "http://localhost:9094/api/v0/pins/$test_cid" &>/dev/null || true
    else
        warning "Pin operation test failed (this might be expected if IPFS content is not available)"
    fi
    
    # Test Prometheus metrics
    log "Testing Prometheus metrics..."
    if curl -s http://localhost:9090/api/v1/query?query=up | jq -r '.status' 2>/dev/null | grep -q "success"; then
        success "Prometheus is collecting metrics"
    else
        warning "Prometheus metrics may not be available yet"
    fi
    
    success "Setup verification completed!"
}

# Display access information
display_access_info() {
    echo ""
    echo "=================================="
    echo "  Development Environment Ready  "
    echo "=================================="
    echo ""
    echo "Services are now running and accessible at:"
    echo ""
    echo "🔗 IPFS-Cluster API:    http://localhost:9094"
    echo "🔗 IPFS API:             http://localhost:5001"
    echo "🔗 IPFS Gateway:         http://localhost:8080"
    echo "🔗 Prometheus:           http://localhost:9090"
    echo "🔗 Grafana:              http://localhost:3000 (admin/admin)"
    echo ""
    echo "📊 ScyllaDB:"
    echo "   - CQL Port:           localhost:9042"
    echo "   - Keyspace:           ipfs_pins"
    echo "   - Connect:            docker exec -it scylla-dev cqlsh"
    echo ""
    echo "📈 Monitoring:"
    echo "   - Grafana Dashboard:  http://localhost:3000/d/ipfs-cluster-scylladb"
    echo "   - Prometheus Targets: http://localhost:9090/targets"
    echo ""
    echo "🧪 Testing:"
    echo "   - Pin a file:         curl -X POST http://localhost:9094/api/v0/pins/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
    echo "   - List pins:          curl http://localhost:9094/api/v0/pins"
    echo "   - Cluster status:     curl http://localhost:9094/api/v0/peers"
    echo ""
    echo "🛠️  Management:"
    echo "   - View logs:          docker-compose logs -f [service_name]"
    echo "   - Stop environment:   docker-compose down"
    echo "   - Reset data:         docker-compose down -v"
    echo ""
    echo "📚 Documentation:"
    echo "   - Examples:           ${PROJECT_ROOT}/examples/"
    echo "   - Configuration:      ${PROJECT_ROOT}/examples/configurations/"
    echo "   - Benchmarks:         ${PROJECT_ROOT}/examples/benchmarks/"
    echo ""
}

# Cleanup function
cleanup_on_error() {
    error "Setup failed. Cleaning up..."
    cd "${PROJECT_ROOT}/examples/docker-compose/development" 2>/dev/null || true
    docker-compose down -v 2>/dev/null || true
}

# Main execution
main() {
    log "Starting IPFS-Cluster ScyllaDB development environment setup..."
    
    # Set up error handling
    trap cleanup_on_error ERR
    
    # Parse command line arguments
    SKIP_VERIFICATION=false
    FORCE_REBUILD=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --skip-verification)
                SKIP_VERIFICATION=true
                shift
                ;;
            --force-rebuild)
                FORCE_REBUILD=true
                shift
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo "Options:"
                echo "  --skip-verification  Skip setup verification"
                echo "  --force-rebuild      Force rebuild of containers"
                echo "  --help               Show this help message"
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Execute setup steps
    check_prerequisites
    setup_development_environment
    
    if [ "$SKIP_VERIFICATION" = false ]; then
        verify_setup
    fi
    
    display_access_info
    
    success "Development environment setup completed successfully!"
    log "You can now start developing and testing IPFS-Cluster with ScyllaDB integration."
}

# Execute main function
main "$@"