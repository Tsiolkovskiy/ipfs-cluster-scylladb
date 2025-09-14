#!/bin/bash

# IPFS-Cluster ScyllaDB Integration - Comprehensive Benchmark Suite
# This script runs all performance benchmarks and generates reports

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORTS_DIR="${SCRIPT_DIR}/reports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_PREFIX="${REPORTS_DIR}/benchmark_${TIMESTAMP}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
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
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Please install Go 1.19 or later."
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    if [[ "$(printf '%s\n' "1.19" "$GO_VERSION" | sort -V | head -n1)" != "1.19" ]]; then
        error "Go version 1.19 or later is required. Current version: $GO_VERSION"
        exit 1
    fi
    
    # Check if Docker is running
    if ! docker info &> /dev/null; then
        error "Docker is not running. Please start Docker."
        exit 1
    fi
    
    # Check if docker-compose is available
    if ! command -v docker-compose &> /dev/null; then
        error "docker-compose is not installed."
        exit 1
    fi
    
    success "Prerequisites check passed"
}

# Setup test environment
setup_environment() {
    log "Setting up test environment..."
    
    # Create reports directory
    mkdir -p "${REPORTS_DIR}"
    
    # Start test environment
    cd "${SCRIPT_DIR}/../docker-compose/testing"
    
    log "Starting ScyllaDB and IPFS-Cluster test environment..."
    docker-compose down -v 2>/dev/null || true
    docker-compose up -d
    
    # Wait for services to be ready
    log "Waiting for services to be ready..."
    sleep 60
    
    # Check ScyllaDB health
    for i in {1..30}; do
        if docker exec scylla-1 cqlsh -e "SELECT now() FROM system.local;" &>/dev/null; then
            success "ScyllaDB is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "ScyllaDB failed to start within timeout"
            exit 1
        fi
        sleep 5
    done
    
    # Check IPFS-Cluster health
    for i in {1..30}; do
        if curl -s http://localhost:9094/api/v0/id &>/dev/null; then
            success "IPFS-Cluster is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "IPFS-Cluster failed to start within timeout"
            exit 1
        fi
        sleep 5
    done
    
    cd "${SCRIPT_DIR}"
}

# Run pin operations benchmark
run_pin_operations_benchmark() {
    log "Running pin operations benchmark..."
    
    cd "${SCRIPT_DIR}/pin-operations"
    
    # Build benchmark tool
    go build -o pin_benchmark ./pin_benchmark.go
    
    # Run different scenarios
    scenarios=(
        "single_pin:1:1000"
        "batch_pin:10:1000"
        "concurrent_pin:50:1000"
        "mixed_operations:25:2000"
    )
    
    for scenario in "${scenarios[@]}"; do
        IFS=':' read -r name concurrency operations <<< "$scenario"
        log "Running scenario: $name (concurrency: $concurrency, operations: $operations)"
        
        ./pin_benchmark \
            -cluster-api="http://localhost:9094" \
            -concurrency="$concurrency" \
            -operations="$operations" \
            -scenario="$name" \
            -output="${REPORT_PREFIX}_pin_${name}.json" \
            -duration="5m"
    done
    
    success "Pin operations benchmark completed"
}

# Run state migration benchmark
run_state_migration_benchmark() {
    log "Running state migration benchmark..."
    
    cd "${SCRIPT_DIR}/state-migration"
    
    # Build migration benchmark tool
    go build -o migration_benchmark ./migration_benchmark.go
    
    # Test migration from different backends
    backends=("inmem" "badger" "leveldb")
    
    for backend in "${backends[@]}"; do
        log "Testing migration from $backend backend..."
        
        ./migration_benchmark \
            -source-backend="$backend" \
            -target-backend="scylladb" \
            -pin-count="10000" \
            -batch-size="1000" \
            -output="${REPORT_PREFIX}_migration_${backend}.json"
    done
    
    success "State migration benchmark completed"
}

# Run load testing
run_load_testing() {
    log "Running load testing..."
    
    cd "${SCRIPT_DIR}/load-testing"
    
    # Build load test tool
    go build -o load_test ./cluster_load_test.go
    
    # Run sustained load test
    log "Running sustained load test..."
    ./load_test \
        -cluster-endpoints="http://localhost:9094,http://localhost:9194,http://localhost:9294" \
        -duration="10m" \
        -pin-rate="100" \
        -unpin-rate="50" \
        -list-rate="200" \
        -output="${REPORT_PREFIX}_load_sustained.json"
    
    # Run spike test
    log "Running spike test..."
    ./load_test \
        -cluster-endpoints="http://localhost:9094,http://localhost:9194,http://localhost:9294" \
        -duration="5m" \
        -pin-rate="500" \
        -unpin-rate="250" \
        -list-rate="1000" \
        -spike-mode=true \
        -output="${REPORT_PREFIX}_load_spike.json"
    
    success "Load testing completed"
}

# Run ScyllaDB performance benchmark
run_scylladb_benchmark() {
    log "Running ScyllaDB performance benchmark..."
    
    cd "${SCRIPT_DIR}/scylladb-performance"
    
    # Build ScyllaDB benchmark tool
    go build -o scylla_benchmark ./scylla_benchmark.go
    
    # Test different consistency levels
    consistency_levels=("ONE" "QUORUM" "ALL")
    
    for consistency in "${consistency_levels[@]}"; do
        log "Testing consistency level: $consistency"
        
        ./scylla_benchmark \
            -hosts="localhost:9042,localhost:9043,localhost:9044" \
            -keyspace="ipfs_pins" \
            -consistency="$consistency" \
            -operations="10000" \
            -concurrency="50" \
            -output="${REPORT_PREFIX}_scylla_${consistency}.json"
    done
    
    success "ScyllaDB performance benchmark completed"
}

# Generate comprehensive report
generate_report() {
    log "Generating comprehensive benchmark report..."
    
    cd "${SCRIPT_DIR}/reports"
    
    # Build report generator
    go build -o report_generator ./report_generator.go
    
    # Generate HTML report
    ./report_generator \
        -input-pattern="${REPORT_PREFIX}_*.json" \
        -output="${REPORT_PREFIX}_comprehensive_report.html" \
        -format="html"
    
    # Generate markdown summary
    ./report_generator \
        -input-pattern="${REPORT_PREFIX}_*.json" \
        -output="${REPORT_PREFIX}_summary.md" \
        -format="markdown"
    
    success "Benchmark report generated: ${REPORT_PREFIX}_comprehensive_report.html"
}

# Cleanup test environment
cleanup_environment() {
    log "Cleaning up test environment..."
    
    cd "${SCRIPT_DIR}/../docker-compose/testing"
    docker-compose down -v
    
    success "Test environment cleaned up"
}

# Main execution
main() {
    log "Starting IPFS-Cluster ScyllaDB benchmark suite..."
    
    # Parse command line arguments
    SKIP_SETUP=false
    SKIP_CLEANUP=false
    BENCHMARKS_TO_RUN="all"
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --skip-setup)
                SKIP_SETUP=true
                shift
                ;;
            --skip-cleanup)
                SKIP_CLEANUP=true
                shift
                ;;
            --benchmarks)
                BENCHMARKS_TO_RUN="$2"
                shift 2
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo "Options:"
                echo "  --skip-setup     Skip environment setup"
                echo "  --skip-cleanup   Skip environment cleanup"
                echo "  --benchmarks     Comma-separated list of benchmarks to run"
                echo "                   Available: pin-ops,migration,load,scylladb,all"
                echo "  --help           Show this help message"
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Check prerequisites
    check_prerequisites
    
    # Setup environment if not skipped
    if [ "$SKIP_SETUP" = false ]; then
        setup_environment
    fi
    
    # Run benchmarks based on selection
    if [[ "$BENCHMARKS_TO_RUN" == "all" || "$BENCHMARKS_TO_RUN" == *"pin-ops"* ]]; then
        run_pin_operations_benchmark
    fi
    
    if [[ "$BENCHMARKS_TO_RUN" == "all" || "$BENCHMARKS_TO_RUN" == *"migration"* ]]; then
        run_state_migration_benchmark
    fi
    
    if [[ "$BENCHMARKS_TO_RUN" == "all" || "$BENCHMARKS_TO_RUN" == *"load"* ]]; then
        run_load_testing
    fi
    
    if [[ "$BENCHMARKS_TO_RUN" == "all" || "$BENCHMARKS_TO_RUN" == *"scylladb"* ]]; then
        run_scylladb_benchmark
    fi
    
    # Generate comprehensive report
    generate_report
    
    # Cleanup environment if not skipped
    if [ "$SKIP_CLEANUP" = false ]; then
        cleanup_environment
    fi
    
    success "Benchmark suite completed successfully!"
    log "Reports available in: ${REPORTS_DIR}"
    log "Main report: ${REPORT_PREFIX}_comprehensive_report.html"
}

# Execute main function
main "$@"