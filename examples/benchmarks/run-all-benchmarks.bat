@echo off
REM IPFS-Cluster ScyllaDB Integration - Comprehensive Benchmark Suite (Windows)
REM This script runs all performance benchmarks and generates reports

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set REPORTS_DIR=%SCRIPT_DIR%reports
set TIMESTAMP=%date:~-4,4%%date:~-10,2%%date:~-7,2%_%time:~0,2%%time:~3,2%%time:~6,2%
set TIMESTAMP=%TIMESTAMP: =0%
set REPORT_PREFIX=%REPORTS_DIR%\benchmark_%TIMESTAMP%

echo [%date% %time%] Starting IPFS-Cluster ScyllaDB benchmark suite...

REM Check prerequisites
echo [%date% %time%] Checking prerequisites...

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go is not installed. Please install Go 1.19 or later.
    exit /b 1
)

REM Check if Docker is running
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running. Please start Docker.
    exit /b 1
)

REM Check if docker-compose is available
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] docker-compose is not installed.
    exit /b 1
)

echo [SUCCESS] Prerequisites check passed

REM Create reports directory
if not exist "%REPORTS_DIR%" mkdir "%REPORTS_DIR%"

REM Setup test environment
echo [%date% %time%] Setting up test environment...
cd /d "%SCRIPT_DIR%\..\docker-compose\testing"

echo [%date% %time%] Starting ScyllaDB and IPFS-Cluster test environment...
docker-compose down -v >nul 2>&1
docker-compose up -d

REM Wait for services to be ready
echo [%date% %time%] Waiting for services to be ready...
timeout /t 60 /nobreak >nul

REM Check ScyllaDB health
echo [%date% %time%] Checking ScyllaDB health...
for /l %%i in (1,1,30) do (
    docker exec scylla-1 cqlsh -e "SELECT now() FROM system.local;" >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] ScyllaDB is ready
        goto :scylla_ready
    )
    timeout /t 5 /nobreak >nul
)
echo [ERROR] ScyllaDB failed to start within timeout
exit /b 1

:scylla_ready

REM Check IPFS-Cluster health
echo [%date% %time%] Checking IPFS-Cluster health...
for /l %%i in (1,1,30) do (
    curl -s http://localhost:9094/api/v0/id >nul 2>&1
    if not errorlevel 1 (
        echo [SUCCESS] IPFS-Cluster is ready
        goto :cluster_ready
    )
    timeout /t 3 /nobreak >nul
)
echo [ERROR] IPFS-Cluster failed to start within timeout
exit /b 1

:cluster_ready

cd /d "%SCRIPT_DIR%"

REM Run pin operations benchmark
echo [%date% %time%] Running pin operations benchmark...
cd /d "%SCRIPT_DIR%\pin-operations"

REM Build benchmark tool
go build -o pin_benchmark.exe pin_benchmark.go

REM Run different scenarios
set scenarios=single_pin:1:1000 batch_pin:10:1000 concurrent_pin:50:1000 mixed_operations:25:2000

for %%s in (%scenarios%) do (
    for /f "tokens=1,2,3 delims=:" %%a in ("%%s") do (
        echo [%date% %time%] Running scenario: %%a (concurrency: %%b, operations: %%c)
        pin_benchmark.exe -cluster-api="http://localhost:9094" -concurrency=%%b -operations=%%c -scenario="%%a" -output="%REPORT_PREFIX%_pin_%%a.json" -duration="5m"
    )
)

echo [SUCCESS] Pin operations benchmark completed

REM Generate simple report
echo [%date% %time%] Generating benchmark report...
echo Benchmark Results > "%REPORT_PREFIX%_summary.txt"
echo ================== >> "%REPORT_PREFIX%_summary.txt"
echo Timestamp: %TIMESTAMP% >> "%REPORT_PREFIX%_summary.txt"
echo. >> "%REPORT_PREFIX%_summary.txt"

for %%f in ("%REPORT_PREFIX%_*.json") do (
    echo Results from %%~nf: >> "%REPORT_PREFIX%_summary.txt"
    type "%%f" | findstr "operations_per_second\|successful_operations\|failed_operations" >> "%REPORT_PREFIX%_summary.txt"
    echo. >> "%REPORT_PREFIX%_summary.txt"
)

REM Cleanup test environment
echo [%date% %time%] Cleaning up test environment...
cd /d "%SCRIPT_DIR%\..\docker-compose\testing"
docker-compose down -v >nul

echo [SUCCESS] Benchmark suite completed successfully!
echo Reports available in: %REPORTS_DIR%
echo Main report: %REPORT_PREFIX%_summary.txt

pause