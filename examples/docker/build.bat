@echo off
REM Build script for IPFS-Cluster with ScyllaDB integration (Windows)
REM This script builds Docker images for Windows environments

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set PROJECT_ROOT=%SCRIPT_DIR%..\..

REM Default values
set IMAGE_NAME=ipfs-cluster-scylladb
set IMAGE_TAG=latest
set DOCKERFILE=Dockerfile.cluster
set PLATFORM=linux/amd64
set PUSH=false
set BUILD_ARGS=

echo [%date% %time%] Building IPFS-Cluster Docker image with ScyllaDB support

REM Parse command line arguments
:parse_args
if "%~1"=="" goto :start_build
if "%~1"=="--name" (
    set IMAGE_NAME=%~2
    shift
    shift
    goto :parse_args
)
if "%~1"=="--tag" (
    set IMAGE_TAG=%~2
    shift
    shift
    goto :parse_args
)
if "%~1"=="--dockerfile" (
    set DOCKERFILE=%~2
    shift
    shift
    goto :parse_args
)
if "%~1"=="--push" (
    set PUSH=true
    shift
    goto :parse_args
)
if "%~1"=="--no-cache" (
    set BUILD_ARGS=%BUILD_ARGS% --no-cache
    shift
    goto :parse_args
)
if "%~1"=="--help" (
    goto :show_help
)
shift
goto :parse_args

:start_build

REM Validate inputs
if not exist "%SCRIPT_DIR%%DOCKERFILE%" (
    echo [ERROR] Dockerfile not found: %SCRIPT_DIR%%DOCKERFILE%
    exit /b 1
)

REM Set build metadata
for /f %%i in ('powershell -Command "Get-Date -UFormat '%%Y-%%m-%%dT%%H:%%M:%%SZ'"') do set BUILD_DATE=%%i
for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set VCS_REF=%%i
if "%VCS_REF%"=="" set VCS_REF=unknown
for /f %%i in ('git describe --tags --always 2^>nul') do set VERSION=%%i
if "%VERSION%"=="" set VERSION=dev

REM Add metadata build args
set BUILD_ARGS=%BUILD_ARGS% --build-arg BUILD_DATE=%BUILD_DATE%
set BUILD_ARGS=%BUILD_ARGS% --build-arg VCS_REF=%VCS_REF%
set BUILD_ARGS=%BUILD_ARGS% --build-arg VERSION=%VERSION%

REM Full image name
set FULL_IMAGE_NAME=%IMAGE_NAME%:%IMAGE_TAG%

echo Image: %FULL_IMAGE_NAME%
echo Platform: %PLATFORM%
echo Dockerfile: %DOCKERFILE%
echo Build context: %PROJECT_ROOT%

REM Build the image
echo [%date% %time%] Starting build process...

cd /d "%PROJECT_ROOT%"

docker build ^
    --platform %PLATFORM% ^
    --file "%SCRIPT_DIR%%DOCKERFILE%" ^
    --tag "%FULL_IMAGE_NAME%" ^
    %BUILD_ARGS% ^
    .

if errorlevel 1 (
    echo [ERROR] Build failed!
    exit /b 1
)

echo [SUCCESS] Build completed successfully!

REM Show image info
echo Image information:
docker images %IMAGE_NAME% --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedAt}}"

REM Push if requested
if "%PUSH%"=="true" (
    echo [%date% %time%] Pushing image to registry...
    docker push "%FULL_IMAGE_NAME%"
    if errorlevel 1 (
        echo [ERROR] Push failed!
        exit /b 1
    )
    echo [SUCCESS] Image pushed successfully!
)

REM Show usage instructions
echo.
echo === Usage Instructions ===
echo Run the built image:
echo   docker run -d --name ipfs-cluster-scylladb \
echo     -p 9094:9094 -p 9095:9095 -p 9096:9096 -p 8888:8888 \
echo     -e SCYLLADB_HOSTS=scylla-host \
echo     -e SCYLLADB_KEYSPACE=ipfs_pins \
echo     %FULL_IMAGE_NAME%
echo.
echo Use with docker-compose:
echo   cd examples\docker-compose\build
echo   docker-compose up -d
echo.

goto :end

:show_help
echo Usage: %0 [OPTIONS]
echo.
echo Build IPFS-Cluster Docker image with ScyllaDB state storage support.
echo.
echo OPTIONS:
echo     --name NAME         Image name (default: %IMAGE_NAME%)
echo     --tag TAG          Image tag (default: %IMAGE_TAG%)
echo     --dockerfile FILE  Dockerfile to use (default: %DOCKERFILE%)
echo     --push             Push image to registry after build
echo     --no-cache         Build without using cache
echo     --help             Show this help message
echo.
echo EXAMPLES:
echo     %0
echo     %0 --tag v1.1.4-scylladb
echo     %0 --tag latest --push
echo.

:end