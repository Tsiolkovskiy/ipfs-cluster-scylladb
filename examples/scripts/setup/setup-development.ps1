# IPFS-Cluster ScyllaDB Integration - Development Environment Setup Script (PowerShell)
# This script sets up a complete development environment for testing ScyllaDB integration

param(
    [switch]$SkipVerification,
    [switch]$ForceRebuild,
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: .\setup-development.ps1 [OPTIONS]"
    Write-Host "Options:"
    Write-Host "  -SkipVerification  Skip setup verification"
    Write-Host "  -ForceRebuild      Force rebuild of containers"
    Write-Host "  -Help              Show this help message"
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path "$ScriptDir\..\..\..\"

function Write-Log {
    param([string]$Message, [string]$Level = "INFO")
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $color = switch ($Level) {
        "ERROR" { "Red" }
        "SUCCESS" { "Green" }
        "WARNING" { "Yellow" }
        default { "Blue" }
    }
    Write-Host "[$timestamp] $Message" -ForegroundColor $color
}

function Test-Prerequisites {
    Write-Log "Checking prerequisites..."
    
    # Check if Docker is installed and running
    try {
        $null = docker --version
        Write-Log "Docker is installed" "SUCCESS"
    }
    catch {
        Write-Log "Docker is not installed. Please install Docker Desktop for Windows." "ERROR"
        exit 1
    }
    
    try {
        $null = docker info 2>$null
        Write-Log "Docker is running" "SUCCESS"
    }
    catch {
        Write-Log "Docker is not running. Please start Docker Desktop." "ERROR"
        exit 1
    }
    
    # Check if docker-compose is available
    try {
        $null = docker-compose --version
        Write-Log "docker-compose is available" "SUCCESS"
    }
    catch {
        Write-Log "docker-compose is not installed. Please install docker-compose." "ERROR"
        exit 1
    }
    
    # Check available disk space (minimum 10GB)
    $drive = Get-WmiObject -Class Win32_LogicalDisk -Filter "DeviceID='C:'"
    $freeSpaceGB = [math]::Round($drive.FreeSpace / 1GB, 2)
    if ($freeSpaceGB -lt 10) {
        Write-Log "Less than 10GB of disk space available ($freeSpaceGB GB). The setup might fail." "WARNING"
    }
    
    Write-Log "Prerequisites check passed" "SUCCESS"
}

function Start-DevelopmentEnvironment {
    Write-Log "Setting up development environment..."
    
    Set-Location "$ProjectRoot\examples\docker-compose\development"
    
    # Stop any existing containers
    Write-Log "Stopping existing containers..."
    docker-compose down -v 2>$null
    
    # Pull latest images
    Write-Log "Pulling latest Docker images..."
    docker-compose pull
    
    # Start the environment
    Write-Log "Starting development environment..."
    docker-compose up -d
    
    # Wait for services to be ready
    Write-Log "Waiting for services to start..."
    
    # Wait for ScyllaDB
    Write-Log "Waiting for ScyllaDB to be ready..."
    $maxAttempts = 60
    for ($i = 1; $i -le $maxAttempts; $i++) {
        try {
            $null = docker exec scylla-dev cqlsh -e "SELECT now() FROM system.local;" 2>$null
            Write-Log "ScyllaDB is ready" "SUCCESS"
            break
        }
        catch {
            if ($i -eq $maxAttempts) {
                Write-Log "ScyllaDB failed to start within timeout" "ERROR"
                return $false
            }
            Start-Sleep -Seconds 5
            Write-Host "." -NoNewline
        }
    }
    
    # Wait for IPFS
    Write-Log "Waiting for IPFS to be ready..."
    $maxAttempts = 30
    for ($i = 1; $i -le $maxAttempts; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:5001/api/v0/version" -UseBasicParsing -TimeoutSec 3 2>$null
            if ($response.StatusCode -eq 200) {
                Write-Log "IPFS is ready" "SUCCESS"
                break
            }
        }
        catch {
            if ($i -eq $maxAttempts) {
                Write-Log "IPFS failed to start within timeout" "ERROR"
                return $false
            }
            Start-Sleep -Seconds 3
            Write-Host "." -NoNewline
        }
    }
    
    # Wait for IPFS-Cluster
    Write-Log "Waiting for IPFS-Cluster to be ready..."
    $maxAttempts = 30
    for ($i = 1; $i -le $maxAttempts; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:9094/api/v0/id" -UseBasicParsing -TimeoutSec 3 2>$null
            if ($response.StatusCode -eq 200) {
                Write-Log "IPFS-Cluster is ready" "SUCCESS"
                break
            }
        }
        catch {
            if ($i -eq $maxAttempts) {
                Write-Log "IPFS-Cluster failed to start within timeout" "ERROR"
                return $false
            }
            Start-Sleep -Seconds 3
            Write-Host "." -NoNewline
        }
    }
    
    # Wait for Prometheus
    Write-Log "Waiting for Prometheus to be ready..."
    $maxAttempts = 20
    for ($i = 1; $i -le $maxAttempts; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:9090/-/ready" -UseBasicParsing -TimeoutSec 3 2>$null
            if ($response.StatusCode -eq 200) {
                Write-Log "Prometheus is ready" "SUCCESS"
                break
            }
        }
        catch {
            if ($i -eq $maxAttempts) {
                Write-Log "Prometheus may not be ready yet" "WARNING"
                break
            }
            Start-Sleep -Seconds 3
            Write-Host "." -NoNewline
        }
    }
    
    # Wait for Grafana
    Write-Log "Waiting for Grafana to be ready..."
    $maxAttempts = 20
    for ($i = 1; $i -le $maxAttempts; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:3000/api/health" -UseBasicParsing -TimeoutSec 3 2>$null
            if ($response.StatusCode -eq 200) {
                Write-Log "Grafana is ready" "SUCCESS"
                break
            }
        }
        catch {
            if ($i -eq $maxAttempts) {
                Write-Log "Grafana may not be ready yet" "WARNING"
                break
            }
            Start-Sleep -Seconds 3
            Write-Host "." -NoNewline
        }
    }
    
    Write-Log "Development environment is ready!" "SUCCESS"
    return $true
}

function Test-Setup {
    Write-Log "Verifying setup..."
    
    # Test ScyllaDB connection
    Write-Log "Testing ScyllaDB connection..."
    try {
        $null = docker exec scylla-dev cqlsh -e "DESCRIBE KEYSPACE ipfs_pins;" 2>$null
        Write-Log "ScyllaDB keyspace created successfully" "SUCCESS"
    }
    catch {
        Write-Log "ScyllaDB keyspace not found" "ERROR"
        return $false
    }
    
    # Test IPFS-Cluster API
    Write-Log "Testing IPFS-Cluster API..."
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:9094/api/v0/id" -UseBasicParsing
        if ($response.StatusCode -eq 200) {
            $clusterId = ($response.Content | ConvertFrom-Json).id
            Write-Log "IPFS-Cluster API is working (ID: $clusterId)" "SUCCESS"
        }
    }
    catch {
        Write-Log "IPFS-Cluster API is not responding correctly" "ERROR"
        return $false
    }
    
    # Test pin operation
    Write-Log "Testing pin operation..."
    try {
        $testCid = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
        $response = Invoke-WebRequest -Uri "http://localhost:9094/api/v0/pins/$testCid" -Method POST -UseBasicParsing 2>$null
        Write-Log "Pin operation test successful" "SUCCESS"
        
        # Clean up test pin
        try {
            Invoke-WebRequest -Uri "http://localhost:9094/api/v0/pins/$testCid" -Method DELETE -UseBasicParsing 2>$null
        }
        catch {
            # Ignore cleanup errors
        }
    }
    catch {
        Write-Log "Pin operation test failed (this might be expected if IPFS content is not available)" "WARNING"
    }
    
    # Test Prometheus metrics
    Write-Log "Testing Prometheus metrics..."
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:9090/api/v1/query?query=up" -UseBasicParsing
        $data = $response.Content | ConvertFrom-Json
        if ($data.status -eq "success") {
            Write-Log "Prometheus is collecting metrics" "SUCCESS"
        }
    }
    catch {
        Write-Log "Prometheus metrics may not be available yet" "WARNING"
    }
    
    Write-Log "Setup verification completed!" "SUCCESS"
    return $true
}

function Show-AccessInfo {
    Write-Host ""
    Write-Host "==================================" -ForegroundColor Green
    Write-Host "  Development Environment Ready  " -ForegroundColor Green
    Write-Host "==================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "Services are now running and accessible at:"
    Write-Host ""
    Write-Host "🔗 IPFS-Cluster API:    http://localhost:9094" -ForegroundColor Cyan
    Write-Host "🔗 IPFS API:             http://localhost:5001" -ForegroundColor Cyan
    Write-Host "🔗 IPFS Gateway:         http://localhost:8080" -ForegroundColor Cyan
    Write-Host "🔗 Prometheus:           http://localhost:9090" -ForegroundColor Cyan
    Write-Host "🔗 Grafana:              http://localhost:3000 (admin/admin)" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "📊 ScyllaDB:" -ForegroundColor Yellow
    Write-Host "   - CQL Port:           localhost:9042"
    Write-Host "   - Keyspace:           ipfs_pins"
    Write-Host "   - Connect:            docker exec -it scylla-dev cqlsh"
    Write-Host ""
    Write-Host "📈 Monitoring:" -ForegroundColor Yellow
    Write-Host "   - Grafana Dashboard:  http://localhost:3000/d/ipfs-cluster-scylladb"
    Write-Host "   - Prometheus Targets: http://localhost:9090/targets"
    Write-Host ""
    Write-Host "🧪 Testing:" -ForegroundColor Yellow
    Write-Host "   - Pin a file:         curl -X POST http://localhost:9094/api/v0/pins/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
    Write-Host "   - List pins:          curl http://localhost:9094/api/v0/pins"
    Write-Host "   - Cluster status:     curl http://localhost:9094/api/v0/peers"
    Write-Host ""
    Write-Host "🛠️  Management:" -ForegroundColor Yellow
    Write-Host "   - View logs:          docker-compose logs -f [service_name]"
    Write-Host "   - Stop environment:   docker-compose down"
    Write-Host "   - Reset data:         docker-compose down -v"
    Write-Host ""
    Write-Host "📚 Documentation:" -ForegroundColor Yellow
    Write-Host "   - Examples:           $ProjectRoot\examples\"
    Write-Host "   - Configuration:      $ProjectRoot\examples\configurations\"
    Write-Host "   - Benchmarks:         $ProjectRoot\examples\benchmarks\"
    Write-Host ""
}

function Invoke-Cleanup {
    Write-Log "Setup failed. Cleaning up..." "ERROR"
    Set-Location "$ProjectRoot\examples\docker-compose\development"
    docker-compose down -v 2>$null
}

# Main execution
try {
    Write-Log "Starting IPFS-Cluster ScyllaDB development environment setup..."
    
    Test-Prerequisites
    
    $success = Start-DevelopmentEnvironment
    if (-not $success) {
        Invoke-Cleanup
        exit 1
    }
    
    if (-not $SkipVerification) {
        $success = Test-Setup
        if (-not $success) {
            Invoke-Cleanup
            exit 1
        }
    }
    
    Show-AccessInfo
    
    Write-Log "Development environment setup completed successfully!" "SUCCESS"
    Write-Log "You can now start developing and testing IPFS-Cluster with ScyllaDB integration."
}
catch {
    Write-Log "An error occurred: $($_.Exception.Message)" "ERROR"
    Invoke-Cleanup
    exit 1
}