param(
    [ValidateSet("all", "server", "client", "clean")]
    [string]$Target = "all"
)

$BINDIR = "bin"
$GOFLAGS = @("-ldflags=-s -w", "-trimpath")

function Build-Server {
    Write-Host "Building tund..." -ForegroundColor Green
    go build @GOFLAGS -o "$BINDIR\tund.exe" .\cmd\server\main.go
    if ($?) { Write-Host "  tund.exe OK" -ForegroundColor Green }
}

function Build-Client {
    Write-Host "Building tun..." -ForegroundColor Green
    go build @GOFLAGS -o "$BINDIR\tun.exe" .\cmd\client\main.go
    if ($?) { Write-Host "  tun.exe OK" -ForegroundColor Green }
}

function Clean {
    Write-Host "Cleaning..." -ForegroundColor Yellow
    if (Test-Path $BINDIR) {
        Remove-Item -Path "$BINDIR\*.exe" -Force -ErrorAction SilentlyContinue
        Write-Host "  cleaned" -ForegroundColor Yellow
    }
}

switch ($Target) {
    "all"    { Build-Server; Build-Client }
    "server" { Build-Server }
    "client" { Build-Client }
    "clean"  { Clean }
}
