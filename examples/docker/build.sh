#!/bin/bash

# Build script for IPFS-Cluster with ScyllaDB integration
# This script builds Docker images for different architectures and deployment scenarios

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Default values
IMAGE_NAME="ipfs-cluster-scylladb"
IMAGE_TAG="latest"
DOCKERFILE="Dockerfile.cluster"
PLATFORM="linux/amd64"
PUSH=false
BUILD_ARGS=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

show_help() {
    cat << EOF
Usage: $0 [OPTIONS]

Build IPFS-Cluster Docker image with ScyllaDB state storage support.

OPTIONS:
    -n, --name NAME         Image name (default: $IMAGE_NAME)
    -t, --tag TAG          Image tag (default: $IMAGE_TAG)
    -f, --dockerfile FILE  Dockerfile to use (default: $DOCKERFILE)
    -p, --platform ARCH    Target platform (default: $PLATFORM)
    --push                 Push image to registry after build
    --multi-arch           Build for multiple architectures
    --no-cache             Build without using cache
    --build-arg ARG        Pass build argument (can be used multiple times)
    -h, --help             Show this help message

EXAMPLES:
    # Build basic image
    $0

    # Build with custom tag
    $0 --tag v1.1.4-scylladb

    # Build for multiple architectures
    $0 --multi-arch --push

    # Build with custom build args
    $0 --build-arg VERSION=1.1.4 --build-arg BUILD_DATE=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Build and push to registry
    $0 --tag latest --push

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--name)
            IMAGE_NAME="$2"
            shift 2
            ;;
        -t|--tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        -f|--dockerfile)
            DOCKERFILE="$2"
            shift 2
            ;;
        -p|--platform)
            PLATFORM="$2"
            shift 2
            ;;
        --push)
            PUSH=true
            shift
            ;;
        --multi-arch)
            PLATFORM="linux/amd64,linux/arm64"
            shift
            ;;
        --no-cache)
            BUILD_ARGS="$BUILD_ARGS --no-cache"
            shift
            ;;
        --build-arg)
            BUILD_ARGS="$BUILD_ARGS --build-arg $2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Validate inputs
if [ ! -f "$SCRIPT_DIR/$DOCKERFILE" ]; then
    error "Dockerfile not found: $SCRIPT_DIR/$DOCKERFILE"
    exit 1
fi

# Set build metadata
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
VCS_REF=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")

# Add metadata build args
BUILD_ARGS="$BUILD_ARGS --build-arg BUILD_DATE=$BUILD_DATE"
BUILD_ARGS="$BUILD_ARGS --build-arg VCS_REF=$VCS_REF"
BUILD_ARGS="$BUILD_ARGS --build-arg VERSION=$VERSION"

# Full image name
FULL_IMAGE_NAME="$IMAGE_NAME:$IMAGE_TAG"

log "Building IPFS-Cluster Docker image with ScyllaDB support"
log "Image: $FULL_IMAGE_NAME"
log "Platform: $PLATFORM"
log "Dockerfile: $DOCKERFILE"
log "Build context: $PROJECT_ROOT"

# Check if buildx is available for multi-platform builds
if [[ "$PLATFORM" == *","* ]]; then
    if ! docker buildx version >/dev/null 2>&1; then
        error "Docker buildx is required for multi-platform builds"
        exit 1
    fi
    
    # Create builder instance if it doesn't exist
    if ! docker buildx inspect cluster-builder >/dev/null 2>&1; then
        log "Creating buildx builder instance..."
        docker buildx create --name cluster-builder --use
    else
        docker buildx use cluster-builder
    fi
    
    BUILD_CMD="docker buildx build"
    if [ "$PUSH" = true ]; then
        BUILD_ARGS="$BUILD_ARGS --push"
    else
        BUILD_ARGS="$BUILD_ARGS --load"
        warning "Multi-arch build without --push will only load the image for current platform"
    fi
else
    BUILD_CMD="docker build"
fi

# Build the image
log "Starting build process..."

cd "$PROJECT_ROOT"

$BUILD_CMD \
    --platform "$PLATFORM" \
    --file "$SCRIPT_DIR/$DOCKERFILE" \
    --tag "$FULL_IMAGE_NAME" \
    $BUILD_ARGS \
    .

if [ $? -eq 0 ]; then
    success "Build completed successfully!"
    
    # Show image info
    if [[ "$PLATFORM" != *","* ]] || [ "$PUSH" = false ]; then
        log "Image information:"
        docker images "$IMAGE_NAME" --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedAt}}"
    fi
    
    # Push if requested and not multi-arch (multi-arch pushes automatically)
    if [ "$PUSH" = true ] && [[ "$PLATFORM" != *","* ]]; then
        log "Pushing image to registry..."
        docker push "$FULL_IMAGE_NAME"
        success "Image pushed successfully!"
    fi
    
    # Show usage instructions
    echo ""
    echo "=== Usage Instructions ==="
    echo "Run the built image:"
    echo "  docker run -d --name ipfs-cluster-scylladb \\"
    echo "    -p 9094:9094 -p 9095:9095 -p 9096:9096 -p 8888:8888 \\"
    echo "    -e SCYLLADB_HOSTS=scylla-host \\"
    echo "    -e SCYLLADB_KEYSPACE=ipfs_pins \\"
    echo "    $FULL_IMAGE_NAME"
    echo ""
    echo "Use with docker-compose:"
    echo "  cd examples/docker-compose/build"
    echo "  docker-compose up -d"
    echo ""
    
else
    error "Build failed!"
    exit 1
fi