# README_PISCADA

# Build docker

    docker build -t piscada/mtx:latest .
    docker push piscada/mtx:latest
    docker compose up

# Run on Windows:

1. Install gstreamer from binaries on internet (.msi installer)

# Debian

sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp
./mediamtx /path/to/mediamtx.yml

## How to build with in Powershell (Windows)

```powershell
    # Clone and cd
    git clone https://github.com/bluenviron/mediamtx
    cd mediamtx
    go generate ./...

    # Produce Windowsy binary: mediamtx.exe file
    $env:CGO_ENABLED = "0"; go build .
```

# How to build Linux amd64 binary: (in powershell and bash)

```powershell
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build .
```

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .

```
