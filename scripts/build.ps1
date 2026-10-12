$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  New-Item -ItemType Directory -Force -Path dist | Out-Null
  $targets = @(
    @{ GOOS='windows'; GOARCH='amd64'; Ext='.exe' },
    @{ GOOS='linux'; GOARCH='amd64'; Ext='' },
    @{ GOOS='linux'; GOARCH='arm64'; Ext='' },
    @{ GOOS='linux'; GOARCH='loong64'; Ext='' }
  )
  foreach ($target in $targets) {
    $env:GOOS = $target.GOOS; $env:GOARCH = $target.GOARCH; $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags='-s -w' -o ("dist/deploytool-$($target.GOOS)-$($target.GOARCH)$($target.Ext)") ./cmd/deploytool
  }
  # Native Windows desktop build: GUI subsystem (no console window), double-click opens WebView2.
  $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
  go build -trimpath -ldflags='-s -w -H windowsgui' -o 'dist/deploytool-desktop.exe' ./cmd/deploytool-desktop
} finally {
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}
