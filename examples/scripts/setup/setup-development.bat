@echo off
REM IPFS-Cluster ScyllaDB Integration - Development Environment Setup Script (Windows)
REM This script sets up a complete development environment for testing ScyllaDB integration

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set PROJECT_ROOT=%SCRIPT_DIR%..\..\..

echo [%date% %time%] Starting IPFS-Cluster ScyllaDB development environment setup...

REM Check prerequisites
echo [%date% %time%] Checking prerequisites...

REM Check if Docker is installed and running
docker --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not installed. Please install Docker Desktop for Windows.
    pause
    exit /b 1
)

docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running. Please start Docker Desktop.
    pause
    exit /b 1
)

REM Check if docker-compose is available
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] docker-compose is not installed. Please install docker-compose.
    pause
    exit /b 1
)

echo [SUCCESS] Prerequisites check passed

REM Setup development environment
echo [%date% %time%] Setting up development environment...
cd /d "%PROJECT_ROOT%\examples\docker-compose\development"

REM Stop any existing containers
echo [%date% %time%] Stopping existing containers...
docker-compose down -v >nul 2>&1

REM Pull latest images
echo [%date% %time%] Pulling latest Docker images...
docker-compose pull

REM Start the environment
echo [%date% %time%] Starting development environment...
docker-compose up -d

REM Wait for services to be ready
echo [%date% %time%] Waiting for services to start...

REM Wait for ScyllaDB
echo [%date% %time%] Waiting for ScyllaDB to be ready...
for /l %%i in (1,1,60) do (
    docker exec scylla-dev cqlsh -e "SELECT now() FROM system.local;" >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] ScyllaDB is ready
        goto :scylla_ready
    )
    timeout /t 5 /nobreak >nul
    echo .
)
echo [ERROR] ScyllaDB failed to start within timeout
goto :cleanup_error

:scylla_ready

REM Wait for IPFS
echo [%date% %time%] Waiting for IPFS to be ready...
for /l %%i in (1,1,30) do (
    curl -s http://localhost:5001/api/v0/version >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] IPFS is ready
        goto :ipfs_ready
    )
    timeout /t 3 /nobreak >nul
    echo .
)
echo [ERROR] IPFS failed to start within timeout
goto :cleanup_error

:ipfs_ready

REM Wait for IPFS-Cluster
echo [%date% %time%] Waiting for IPFS-Cluster to be ready...
for /l %%i in (1,1,30) do (
    curl -s http://localhost:9094/api/v0/id >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] IPFS-Cluster is ready
        goto :cluster_ready
    )
    timeout /t 3 /nobreak >nul
    echo .
)
echo [ERROR] IPFS-Cluster failed to start within timeout
goto :cleanup_error

:cluster_ready

REM Wait for Prometheus
echo [%date% %time%] Waiting for Prometheus to be ready...
for /l %%i in (1,1,20) do (
    curl -s http://localhost:9090/-/ready >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] Prometheus is ready
        goto :prometheus_ready
    )
    timeout /t 3 /nobreak >nul
    echo .
)
echo [WARNING] Prometheus may not be ready yet

:prometheus_ready

REM Wait for Grafana
echo [%date% %time%] Waiting for Grafana to be ready...
for /l %%i in (1,1,20) do (
    curl -s http://localhost:3000/api/health >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] Grafana is ready
        goto :grafana_ready
    )
    timeout /t 3 /nobreak >nul
    echo .
)
echo [WARNING] Grafana may not be ready yet

:grafana_ready

echo [SUCCESS] Development environment is ready!

REM Verify setup
echo [%date% %time%] Verifying setup...

REM Test ScyllaDB connection
echo [%date% %time%] Testing ScyllaDB connection...
docker exec scylla-dev cqlsh -e "DESCRIBE KEYSPACE ipfs_pins;" >nul 2>&1
if not errorlevel 1 (
    echo [SUCCESS] ScyllaDB keyspace created successfully
) else (
    echo [ERROR] ScyllaDB keyspace not found
    goto :cleanup_error
)

REM Test IPFS-Cluster API
echo [%date% %time%] Testing IPFS-Cluster API...
for /f %%i in ('curl -s http://localhost:9094/api/v0/id') do set cluster_response=%%i
if not "%cluster_response%"=="" (
    echo [SUCCESS] IPFS-Cluster API is working
) else (
    echo [ERROR] IPFS-Cluster API is not responding correctly
    goto :cleanup_error
)

echo [SUCCESS] Setup verification completed!

REM Display access information
echo.
echo ==================================
echo   Development Environment Ready  
echo ==================================
echo.
echo Services are now running and accessible at:
echo.
echo 🔗 IPFS-Cluster API:    http://localhost:9094
echo 🔗 IPFS API:             http://localhost:5001
echo 🔗 IPFS Gateway:         http://localhost:8080
echo 🔗 Prometheus:           http://localhost:9090
echo 🔗 Grafana:              http://localhost:3000 (admin/admin)
echo.
echo 📊 ScyllaDB:
echo    - CQL Port:           localhost:9042
echo    - Keyspace:           ipfs_pins
echo    - Connect:            docker exec -it scylla-dev cqlsh
echo.
echo 📈 Monitoring:
echo    - Grafana Dashboard:  http://localhost:3000/d/ipfs-cluster-scylladb
echo    - Prometheus Targets: http://localhost:9090/targets
echo.
echo 🧪 Testing:
echo    - Pin a file:         curl -X POST http://localhost:9094/api/v0/pins/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG
echo    - List pins:          curl http://localhost:9094/api/v0/pins
echo    - Cluster status:     curl http://localhost:9094/api/v0/peers
echo.
echo 🛠️  Management:
echo    - View logs:          docker-compose logs -f [service_name]
echo    - Stop environment:   docker-compose down
echo    - Reset data:         docker-compose down -v
echo.
echo 📚 Documentation:
echo    - Examples:           %PROJECT_ROOT%\examples\
echo    - Configuration:      %PROJECT_ROOT%\examples\configurations\
echo    - Benchmarks:         %PROJECT_ROOT%\examples\benchmarks\
echo.

echo [SUCCESS] Development environment setup completed successfully!
echo You can now start developing and testing IPFS-Cluster with ScyllaDB integration.
pause
exit /b 0

:cleanup_error
echo [ERROR] Setup failed. Cleaning up...
docker-compose down -v >nul 2>&1
pause
exit /b 1