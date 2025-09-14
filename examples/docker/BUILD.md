# Building IPFS-Cluster with ScyllaDB Integration

This guide explains how to build custom Docker images for IPFS-Cluster with ScyllaDB state storage support.

## Quick Start

### Build with Default Settings

**Linux/macOS:**
```bash
cd examples/docker
chmod +x build.sh
./build.sh
```

**Windows (Command Prompt):**
```cmd
cd examples\docker
build.bat
```

**Windows (PowerShell):**
```powershell
cd examples\docker
.\build.ps1
```

**Windows (Git Bash) - If Docker PATH is configured:**
```bash
cd examples/docker
./build.sh
```

> **Note for Windows users**: If you get "docker: command not found" in Git Bash, see [Windows Docker Setup Guide](WINDOWS_DOCKER_SETUP.md) for solutions.

### Build and Run with Docker Compose

```bash
cd examples/docker-compose/build
docker-compose up --build -d
```

## Build Options

### Basic Build Commands

**Build with custom tag:**
```bash
./build.sh --tag v1.1.4-scylladb
```

**Build without cache:**
```bash
./build.sh --no-cache
```

**Build and push to registry:**
```bash
./build.sh --tag latest --push
```

### Multi-Architecture Builds

**Build for multiple platforms:**
```bash
./build.sh --multi-arch --push
```

**Build for specific platform:**
```bash
./build.sh --platform linux/arm64
```

### Custom Build Arguments

**Pass build-time variables:**
```bash
./build.sh \
  --build-arg VERSION=1.1.4 \
  --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --build-arg ENABLE_DEBUG=true
```

## Dockerfile Structure

### Multi-Stage Build Process

The build process uses a multi-stage Dockerfile:

1. **Builder Stage** (`golang:1.21-alpine`):
   - Installs build dependencies
   - Downloads Go modules
   - Compiles IPFS-Cluster binaries with ScyllaDB support
   - Creates statically linked binaries

2. **Runtime Stage** (`alpine:3.18`):
   - Minimal runtime environment
   - Copies compiled binaries
   - Sets up user permissions
   - Configures entrypoint

### Key Features

- **Static Binaries**: No external dependencies in runtime
- **Security**: Non-root user execution
- **Health Checks**: Built-in container health monitoring
- **Configuration**: Environment variable support
- **Monitoring**: Metrics endpoint exposure

## Configuration Options

### Environment Variables

The built image supports extensive configuration through environment variables:

#### ScyllaDB Configuration
```bash
SCYLLADB_HOSTS=scylla1,scylla2,scylla3
SCYLLADB_PORT=9042
SCYLLADB_KEYSPACE=ipfs_pins
SCYLLADB_USERNAME=cluster_user
SCYLLADB_PASSWORD=secure_password
SCYLLADB_CONSISTENCY=QUORUM
SCYLLADB_NUM_CONNS=10
```

#### Cluster Configuration
```bash
CLUSTER_PEERNAME=my-cluster-node
CLUSTER_SECRET=shared_cluster_secret
CLUSTER_BOOTSTRAP_PEERS=/ip4/10.0.1.10/tcp/9096/p2p/12D3...
CLUSTER_LOGLEVEL=info
```

#### API Configuration
```bash
CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS=/ip4/0.0.0.0/tcp/9094
CLUSTER_IPFSPROXY_LISTENMULTIADDRESS=/ip4/0.0.0.0/tcp/9095
```

### Configuration Templates

Use pre-built configuration templates:

```bash
docker run -d \
  -e CLUSTER_CONFIG_TEMPLATE=multi-node/cluster.json \
  -e SCYLLADB_HOSTS=scylla1,scylla2,scylla3 \
  ipfs-cluster-scylladb:latest
```

Available templates:
- `single-node/cluster.json` - Development setup
- `multi-node/cluster.json` - Production cluster
- `enterprise/cluster.json` - Enterprise deployment

## Running the Built Image

### Basic Run Command

```bash
docker run -d \
  --name ipfs-cluster-scylladb \
  -p 9094:9094 \
  -p 9095:9095 \
  -p 9096:9096 \
  -p 8888:8888 \
  -e SCYLLADB_HOSTS=scylla-host \
  -e SCYLLADB_KEYSPACE=ipfs_pins \
  -v cluster_data:/data/ipfs-cluster \
  ipfs-cluster-scylladb:latest
```

### With Docker Compose

Create a `docker-compose.yml`:

```yaml
version: '3.8'

services:
  scylla:
    image: scylladb/scylla:5.2
    command: --seeds=scylla --smp 1 --memory 1G
    ports:
      - "9042:9042"

  cluster:
    image: ipfs-cluster-scylladb:latest
    depends_on:
      - scylla
    ports:
      - "9094:9094"
      - "9095:9095"
      - "9096:9096"
      - "8888:8888"
    environment:
      - SCYLLADB_HOSTS=scylla
      - SCYLLADB_KEYSPACE=ipfs_pins
      - CLUSTER_LOGLEVEL=info
    volumes:
      - cluster_data:/data/ipfs-cluster

volumes:
  cluster_data:
```

## Development Builds

### Local Development

For development with live code changes:

```bash
# Build development image
./build.sh --tag dev --build-arg ENABLE_DEBUG=true

# Run with source code mounted
docker run -d \
  --name cluster-dev \
  -p 9094:9094 \
  -v $(pwd):/src \
  -e CLUSTER_LOGLEVEL=debug \
  ipfs-cluster-scylladb:dev
```

### Debug Builds

Enable debug features:

```bash
./build.sh \
  --tag debug \
  --build-arg CGO_ENABLED=1 \
  --build-arg GCFLAGS="-N -l" \
  --build-arg ENABLE_RACE_DETECTOR=true
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build and Push Docker Image

on:
  push:
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2
      
      - name: Login to Registry
        uses: docker/login-action@v2
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Build and Push
        run: |
          cd examples/docker
          ./build.sh \
            --name ghcr.io/${{ github.repository }}/ipfs-cluster-scylladb \
            --tag ${{ github.ref_name }} \
            --multi-arch \
            --push
```

### GitLab CI Example

```yaml
build-image:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  script:
    - cd examples/docker
    - chmod +x build.sh
    - ./build.sh --tag $CI_COMMIT_TAG --push
  only:
    - tags
```

## Performance Optimization

### Build Performance

**Use BuildKit for faster builds:**
```bash
export DOCKER_BUILDKIT=1
./build.sh
```

**Multi-stage caching:**
```bash
./build.sh --build-arg BUILDKIT_INLINE_CACHE=1
```

### Runtime Performance

**Optimize for production:**
```bash
./build.sh \
  --build-arg CGO_ENABLED=0 \
  --build-arg LDFLAGS="-w -s" \
  --build-arg TAGS="netgo osusergo static_build"
```

## Troubleshooting

### Common Build Issues

**Go module download failures:**
```bash
# Use Go proxy
./build.sh --build-arg GOPROXY=https://proxy.golang.org,direct
```

**Cross-compilation issues:**
```bash
# Disable CGO for pure Go builds
./build.sh --build-arg CGO_ENABLED=0
```

**Large image size:**
```bash
# Use distroless base image
./build.sh --dockerfile Dockerfile.distroless
```

### Runtime Issues

**Configuration not applied:**
- Check environment variables are set correctly
- Verify configuration template exists
- Check file permissions in container

**ScyllaDB connection failures:**
- Ensure ScyllaDB is accessible from container
- Verify network connectivity
- Check authentication credentials

**Performance issues:**
- Allocate sufficient resources to container
- Tune ScyllaDB connection pool settings
- Monitor container metrics

### Debug Commands

**Inspect built image:**
```bash
docker run --rm -it ipfs-cluster-scylladb:latest sh
```

**Check configuration:**
```bash
docker run --rm ipfs-cluster-scylladb:latest cat /etc/ipfs-cluster/configurations/single-node/cluster.json
```

**View logs:**
```bash
docker logs -f container-name
```

## Security Considerations

### Image Security

- Uses non-root user (`cluster:1000`)
- Minimal attack surface with Alpine base
- No unnecessary packages installed
- Static binaries reduce dependencies

### Runtime Security

```bash
# Run with security options
docker run -d \
  --name cluster-secure \
  --user 1000:1000 \
  --read-only \
  --tmpfs /tmp \
  --cap-drop ALL \
  --cap-add NET_BIND_SERVICE \
  --security-opt no-new-privileges:true \
  ipfs-cluster-scylladb:latest
```

### Network Security

```bash
# Use custom network
docker network create cluster-net
docker run -d --network cluster-net ipfs-cluster-scylladb:latest
```

## Registry Management

### Tagging Strategy

```bash
# Semantic versioning
./build.sh --tag v1.1.4-scylladb
./build.sh --tag v1.1-scylladb
./build.sh --tag v1-scylladb
./build.sh --tag latest

# Feature branches
./build.sh --tag feature-scylladb-optimization

# Environment specific
./build.sh --tag staging
./build.sh --tag production
```

### Multi-Registry Push

```bash
# Push to multiple registries
for registry in docker.io ghcr.io quay.io; do
  docker tag ipfs-cluster-scylladb:latest $registry/myorg/ipfs-cluster-scylladb:latest
  docker push $registry/myorg/ipfs-cluster-scylladb:latest
done
```

## Monitoring and Observability

### Built-in Metrics

The image exposes metrics on port 8888:

```bash
curl http://localhost:8888/debug/metrics/prometheus
```

### Health Checks

```bash
# Check container health
docker inspect --format='{{.State.Health.Status}}' container-name

# Manual health check
docker exec container-name ipfs-cluster-ctl --host /ip4/127.0.0.1/tcp/9094 id
```

### Logging

```bash
# Structured logging
docker run -d \
  -e CLUSTER_LOGLEVEL=info \
  -e LOG_FORMAT=json \
  ipfs-cluster-scylladb:latest
```