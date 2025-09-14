# Windows Docker Setup Guide

This guide helps you set up Docker on Windows and resolve common issues when building IPFS-Cluster images.

## Prerequisites

### 1. Install Docker Desktop for Windows

1. **Download Docker Desktop**: https://www.docker.com/products/docker-desktop
2. **System Requirements**:
   - Windows 10 64-bit: Pro, Enterprise, or Education (Build 15063 or later)
   - Windows 11 64-bit: Home or Pro version 21H2 or higher
   - WSL 2 feature enabled
   - Hyper-V enabled (for Windows Pro/Enterprise)

3. **Installation Steps**:
   - Run the installer as Administrator
   - Follow the installation wizard
   - Restart your computer when prompted
   - Start Docker Desktop from the Start menu

### 2. Verify Docker Installation

Open Command Prompt or PowerShell and run:

```cmd
docker --version
docker-compose --version
```

You should see version information for both commands.

## Common Issues and Solutions

### Issue 1: "docker: command not found" in Git Bash

**Problem**: Docker is not in the PATH for Git Bash (MINGW64).

**Solutions**:

**Option A: Use Command Prompt or PowerShell (Recommended)**
```cmd
# Command Prompt
cd examples\docker
build.bat --tag latest

# PowerShell  
cd examples\docker
.\build.ps1 -Tag latest
```

**Option B: Add Docker to Git Bash PATH**
```bash
# Add to ~/.bashrc or run each time
export PATH="/c/Program Files/Docker/Docker/resources/bin:$PATH"
export PATH="/c/Program Files/Docker/Docker/resources/cli-plugins:$PATH"

# Reload bash configuration
source ~/.bashrc

# Test Docker
docker --version
```

**Option C: Use winpty in Git Bash**
```bash
# Use winpty prefix for Docker commands
winpty docker --version
winpty docker build --help
```

### Issue 2: Docker Desktop Not Running

**Problem**: Docker daemon is not running.

**Solution**:
1. Start Docker Desktop from the Start menu
2. Wait for Docker to fully start (whale icon in system tray)
3. Check Docker Desktop dashboard shows "Engine running"

### Issue 3: WSL 2 Issues

**Problem**: WSL 2 backend issues on Windows.

**Solutions**:

**Enable WSL 2**:
```powershell
# Run as Administrator in PowerShell
dism.exe /online /enable-feature /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart
dism.exe /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart

# Restart computer, then set WSL 2 as default
wsl --set-default-version 2
```

**Update WSL 2 kernel**:
- Download from: https://aka.ms/wsl2kernel
- Install the update package

### Issue 4: Hyper-V Conflicts

**Problem**: Hyper-V conflicts with other virtualization software.

**Solutions**:
- Use WSL 2 backend instead of Hyper-V
- In Docker Desktop Settings → General → Use WSL 2 based engine
- Disable conflicting virtualization software (VirtualBox, VMware)

### Issue 5: Build Performance Issues

**Problem**: Slow Docker builds on Windows.

**Solutions**:

**Optimize Docker Desktop**:
1. Docker Desktop Settings → Resources
2. Increase CPU and Memory allocation
3. Enable "Use Docker Compose V2"

**Use WSL 2 backend**:
- Better performance than Hyper-V
- Native Linux filesystem performance

**Exclude from Windows Defender**:
- Add Docker installation directory to exclusions
- Add project directory to exclusions

## Build Methods for Windows

### Method 1: PowerShell Script (Recommended)

```powershell
# Navigate to docker directory
cd examples\docker

# Build with PowerShell script
.\build.ps1 -Tag "latest"

# Build with custom options
.\build.ps1 -Tag "v1.1.4" -Push -BuildArg @("VERSION=1.1.4")
```

### Method 2: Command Prompt Batch Script

```cmd
cd examples\docker
build.bat --tag latest
```

### Method 3: Direct Docker Commands

```cmd
# Navigate to project root
cd C:\path\to\ipfs-cluster-scylladb

# Build directly with docker
docker build -f examples\docker\Dockerfile.cluster -t ipfs-cluster-scylladb:latest .
```

### Method 4: Docker Compose

```cmd
cd examples\docker-compose\build
docker-compose up --build -d
```

## Environment Setup

### Set Environment Variables

**Command Prompt**:
```cmd
set DOCKER_BUILDKIT=1
set COMPOSE_DOCKER_CLI_BUILD=1
```

**PowerShell**:
```powershell
$env:DOCKER_BUILDKIT = "1"
$env:COMPOSE_DOCKER_CLI_BUILD = "1"
```

**Permanent (System Environment Variables)**:
1. Open System Properties → Advanced → Environment Variables
2. Add new system variables:
   - `DOCKER_BUILDKIT` = `1`
   - `COMPOSE_DOCKER_CLI_BUILD` = `1`

### Configure Git Bash (Optional)

Add to `~/.bashrc`:
```bash
# Docker aliases for Git Bash
alias docker='winpty docker'
alias docker-compose='winpty docker-compose'

# Add Docker to PATH
export PATH="/c/Program Files/Docker/Docker/resources/bin:$PATH"
export PATH="/c/Program Files/Docker/Docker/resources/cli-plugins:$PATH"

# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
```

## Troubleshooting Commands

### Check Docker Status
```cmd
docker info
docker version
docker-compose version
```

### Test Docker Functionality
```cmd
# Test basic Docker functionality
docker run hello-world

# Test Docker Compose
docker-compose --version
```

### View Docker Desktop Logs
1. Open Docker Desktop
2. Click on "Troubleshoot" (bug icon)
3. View logs or create diagnostic bundle

### Reset Docker Desktop
1. Docker Desktop Settings → Troubleshoot
2. Click "Reset to factory defaults"
3. Restart Docker Desktop

## Performance Optimization

### Docker Desktop Settings

**Resources**:
- CPU: Allocate 50-75% of available cores
- Memory: Allocate 4-8GB (or 50% of available RAM)
- Swap: 1-2GB
- Disk image size: 64GB or more

**Advanced**:
- Enable "Use Docker Compose V2"
- Enable "Send usage statistics" (optional)

### Windows-Specific Optimizations

**File System Performance**:
- Use WSL 2 backend for better I/O performance
- Store project files in WSL 2 filesystem when possible
- Avoid deep directory nesting

**Antivirus Exclusions**:
Add these directories to Windows Defender exclusions:
- `C:\Program Files\Docker`
- `C:\ProgramData\Docker`
- `%USERPROFILE%\.docker`
- Your project directory

## Alternative Solutions

### If Docker Desktop Doesn't Work

**Option 1: Use Docker in WSL 2**
```bash
# Install Docker in WSL 2 Ubuntu
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Build from WSL 2
cd /mnt/c/path/to/project
./examples/docker/build.sh --tag latest
```

**Option 2: Use Remote Docker**
- Use Docker on a Linux VM or remote server
- Connect via Docker context or SSH

**Option 3: Use GitHub Actions/CI**
- Set up automated builds in GitHub Actions
- Pull pre-built images instead of building locally

## Getting Help

### Docker Desktop Support
- Docker Desktop → Help → Get Support
- Docker Community Forums: https://forums.docker.com/
- Docker Documentation: https://docs.docker.com/

### Windows-Specific Resources
- WSL 2 Documentation: https://docs.microsoft.com/en-us/windows/wsl/
- Hyper-V Documentation: https://docs.microsoft.com/en-us/virtualization/hyper-v-on-windows/

### Project-Specific Help
- Check `examples/docker/BUILD.md` for detailed build instructions
- Review `examples/WINDOWS_QUICKSTART.md` for Windows-specific guidance
- Open an issue in the project repository