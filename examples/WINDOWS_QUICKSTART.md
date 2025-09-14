# Windows Quick Start Guide

This guide helps you get started with IPFS-Cluster ScyllaDB integration on Windows.

## Prerequisites

1. **Docker Desktop for Windows** - [Download here](https://www.docker.com/products/docker-desktop)
2. **Go 1.19+** (optional, for running benchmarks) - [Download here](https://golang.org/dl/)
3. **Git for Windows** (optional) - [Download here](https://git-scm.github.io/downloads)

## Quick Setup

### Option 1: Automated Setup (Recommended)

**Using Command Prompt:**
```cmd
cd examples\scripts\setup
setup-development.bat
```

**Using PowerShell:**
```powershell
cd examples\scripts\setup
.\setup-development.ps1
```

### Option 2: Manual Setup

1. **Start the development environment:**
   ```cmd
   cd examples\docker-compose\development
   docker-compose up -d
   ```

2. **Wait for services to start** (about 2-3 minutes)

3. **Verify services are running:**
   ```cmd
   docker-compose ps
   ```

4. **Access the services:**
   - Grafana Dashboard: http://localhost:3000 (admin/admin)
   - IPFS-Cluster API: http://localhost:9094
   - Prometheus: http://localhost:9090

## Testing the Setup

### Basic API Tests

**Test cluster status:**
```cmd
curl http://localhost:9094/api/v0/id
```

**Pin a test file:**
```cmd
curl -X POST "http://localhost:9094/api/v0/pins/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
```

**List all pins:**
```cmd
curl http://localhost:9094/api/v0/pins
```

### Database Verification

**Connect to ScyllaDB:**
```cmd
docker exec -it scylla-dev cqlsh
```

**Check the data:**
```sql
USE ipfs_pins;
SELECT * FROM pins_by_cid LIMIT 10;
```

## Running Benchmarks

### Simple Benchmark (PowerShell)

```powershell
cd examples\benchmarks\pin-operations
go run pin_benchmark.go -cluster-api="http://localhost:9094" -scenario="single_pin" -operations=100
```

### Full Benchmark Suite (Command Prompt)

```cmd
cd examples\benchmarks
run-all-benchmarks.bat
```

## Monitoring

### Grafana Dashboard

1. Open http://localhost:3000 in your browser
2. Login with username: `admin`, password: `admin`
3. Navigate to the "IPFS-Cluster ScyllaDB Integration" dashboard
4. Monitor real-time metrics for:
   - Pin operations
   - ScyllaDB performance
   - System resources

### Key Metrics to Watch

- **Total Pins**: Current number of pins in the cluster
- **Pin Operations Rate**: Operations per second
- **ScyllaDB Query Latency**: Database response times
- **Error Rates**: Failed operations

## Troubleshooting

### Common Issues

**Docker not starting:**
- Make sure Docker Desktop is running
- Check if Hyper-V is enabled (Windows Pro/Enterprise)
- Try restarting Docker Desktop

**Services not responding:**
- Wait longer for services to start (up to 5 minutes)
- Check Docker logs: `docker-compose logs [service_name]`
- Restart the environment: `docker-compose restart`

**Port conflicts:**
- Make sure ports 3000, 5001, 8080, 9042, 9090, 9094 are not in use
- Stop other services using these ports
- Modify docker-compose.yml to use different ports if needed

**Performance issues:**
- Allocate more resources to Docker Desktop
- Close other resource-intensive applications
- Check available disk space (need at least 10GB)

### Getting Help

**View service logs:**
```cmd
docker-compose logs -f [service_name]
```

**Check service status:**
```cmd
docker-compose ps
```

**Restart a specific service:**
```cmd
docker-compose restart [service_name]
```

**Reset everything:**
```cmd
docker-compose down -v
docker-compose up -d
```

## Stopping the Environment

**Stop services (keep data):**
```cmd
docker-compose stop
```

**Stop and remove everything:**
```cmd
docker-compose down -v
```

## Next Steps

1. **Explore Configurations**: Check `examples\configurations\` for different deployment scenarios
2. **Run Load Tests**: Use the benchmark tools to test performance
3. **Monitor Performance**: Set up alerts in Grafana for production monitoring
4. **Scale Up**: Try the multi-node setup in `examples\docker-compose\testing\`

## Windows-Specific Notes

- Use backslashes (`\`) in file paths instead of forward slashes (`/`)
- PowerShell and Command Prompt have different syntax for some commands
- Docker Desktop for Windows requires Hyper-V or WSL2
- File permissions work differently - no need for `chmod` commands
- Use `start` command to open URLs: `start http://localhost:3000`

## Performance Tips for Windows

1. **Use WSL2 backend** in Docker Desktop for better performance
2. **Allocate sufficient resources** to Docker Desktop (4GB+ RAM recommended)
3. **Use SSD storage** for Docker volumes
4. **Close unnecessary applications** to free up resources
5. **Enable hardware acceleration** if available