# Build script for IPFS-Cluster with ScyllaDB integration (PowerShell)
# This script builds Docker images for Windows environments

param(
    [string]$Name = "ipfs-cluster-scylladb",
    [string]$Tag = "latest", 
    [string]$Dockerfile = "Dockerfile.cluster",
    [string]$Platform = "linux/amd64",
    [switch]$Push,
    [switch]$MultiArch,
    [switch]$NoCache,
    [string[]]$BuildArg = @(),
    [switch]$Help
)

if ($Help) {
    Write-Host @"
Usage: .\build.ps1 [OPTIONS]

Build IPFS-Cluster Docker image with ScyllaDB state storage support.

OPTIONS:
    -Name NAME         Image name (default: $Name)
    -Tag TAG          Image tag (default: $Tag)
    -Dockerfile FILE  Dockerfile to use (default: $Dockerfile)
    -Platform ARCH    Target platform (default: $Platform)
    -Push             Push image to registry after build
    -MultiArch        Build for multiple architectures
    -NoCache          Build without using cache
    -BuildArg ARG     Pass build argument (can be used multiple times)
    -Help             Show this help message

EXAMPLES:
    # Build basic image
    .\build.ps1

    # Build with custom tag
    .\build.ps1 -Tag "v1.1.4-scylladb"

    # Build for multiple architectures
    .\build.ps1 -MultiArch -Push

    # Build with custom build args
    .\build.ps1 -BuildArg @("VERSION=1.1.4", "BUILD_DATE=$(Get-Date -UFormat '%Y-%m-%dT%H:%M:%SZ')")

    # Build and push to registry
    .\build.ps1 -Tag "latest" -Push
"@
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path "$ScriptDir\..\.."

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

# Validate inputs
if (-not (Test-Path "$ScriptDir\$Dockerfile")) {
    Write-Log "Dockerfile not found: $ScriptDir\$Dockerfile" "ERROR"
    exit 1
}

# Check if Docker is available
try {
    $null = docker --version
    Write-Log "Docker is available" "SUCCESS"
}
catch {
    Write-Log "Docker is not available. Please install Docker Desktop for Windows." "ERROR"
    Write-Log "Download from: https://www.docker.com/products/docker-desktop" "INFO"
    exit 1
}

# Set build metadata
$BuildDate = Get-Date -UFormat "%Y-%m-%dT%H:%M:%SZ"
try {
    $VcsRef = git rev-parse --short HEAD 2>$null
    if (-not $VcsRef) { $VcsRef = "unknown" }
}
catch {
    $VcsRef = "unknown"
}

try {
    $Version = git describe --tags --always 2>$null
    if (-not $Version) { $Version = "dev" }
}
catch {
    $Version = "dev"
}

# Prepare build arguments
$BuildArgs = @(
    "--build-arg", "BUILD_DATE=$BuildDate",
    "--build-arg", "VCS_REF=$VcsRef", 
    "--build-arg", "VERSION=$Version"
)

foreach ($arg in $BuildArg) {
    $BuildArgs += "--build-arg", $arg
}

if ($NoCache) {
    $BuildArgs += "--no-cache"
}

if ($MultiArch) {
    $Platform = "linux/amd64,linux/arm64"
}

# Full image name
$FullImageName = "${Name}:${Tag}"

Write-Log "Building IPFS-Cluster Docker image with ScyllaDB support"
Write-Log "Image: $FullImageName"
Write-Log "Platform: $Platform"
Write-Log "Dockerfile: $Dockerfile"
Write-Log "Build context: $ProjectRoot"

# Determine build command
if ($Platform -like "*,*") {
    # Multi-platform build
    try {
        $null = docker buildx version
        Write-Log "Docker buildx is available for multi-platform builds" "SUCCESS"
    }
    catch {
        Write-Log "Docker buildx is required for multi-platform builds" "ERROR"
        exit 1
    }
    
    # Create builder instance if it doesn't exist
    $builderExists = docker buildx inspect cluster-builder 2>$null
    if (-not $builderExists) {
        Write-Log "Creating buildx builder instance..."
        docker buildx create --name cluster-builder --use
    }
    else {
        docker buildx use cluster-builder
    }
    
    $BuildCmd = "docker", "buildx", "build"
    if ($Push) {
        $BuildArgs += "--push"
    }
    else {
        $BuildArgs += "--load"
        Write-Log "Multi-arch build without -Push will only load the image for current platform" "WARNING"
    }
}
else {
    $BuildCmd = "docker", "build"
}

# Build the image
Write-Log "Starting build process..."

Set-Location $ProjectRoot

$BuildCommand = $BuildCmd + @(
    "--platform", $Platform,
    "--file", "$ScriptDir\$Dockerfile",
    "--tag", $FullImageName
) + $BuildArgs + @(".")

Write-Log "Executing: $($BuildCommand -join ' ')"

try {
    & $BuildCommand[0] $BuildCommand[1..($BuildCommand.Length-1)]
    
    if ($LASTEXITCODE -eq 0) {
        Write-Log "Build completed successfully!" "SUCCESS"
        
        # Show image info
        if (-not ($Platform -like "*,*") -or -not $Push) {
            Write-Log "Image information:"
            docker images $Name --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedAt}}"
        }
        
        # Push if requested and not multi-arch (multi-arch pushes automatically)
        if ($Push -and -not ($Platform -like "*,*")) {
            Write-Log "Pushing image to registry..."
            docker push $FullImageName
            if ($LASTEXITCODE -eq 0) {
                Write-Log "Image pushed successfully!" "SUCCESS"
            }
            else {
                Write-Log "Push failed!" "ERROR"
                exit 1
            }
        }
        
        # Show usage instructions
        Write-Host ""
        Write-Host "=== Usage Instructions ===" -ForegroundColor Green
        Write-Host "Run the built image:"
        Write-Host "  docker run -d --name ipfs-cluster-scylladb \"
        Write-Host "    -p 9094:9094 -p 9095:9095 -p 9096:9096 -p 8888:8888 \"
        Write-Host "    -e SCYLLADB_HOSTS=scylla-host \"
        Write-Host "    -e SCYLLADB_KEYSPACE=ipfs_pins \"
        Write-Host "    $FullImageName"
        Write-Host ""
        Write-Host "Use with docker-compose:"
        Write-Host "  cd examples\docker-compose\build"
        Write-Host "  docker-compose up -d"
        Write-Host ""
    }
    else {
        Write-Log "Build failed!" "ERROR"
        exit 1
    }
}
catch {
    Write-Log "Build failed with error: $($_.Exception.Message)" "ERROR"
    exit 1
}