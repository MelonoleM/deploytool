#!/usr/bin/env sh
set -eu
mkdir -p dist
for target in windows/amd64 linux/amd64 linux/arm64 linux/loong64; do
  os=${target%/*}; arch=${target#*/}; ext=""
  [ "$os" = windows ] && ext=.exe
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "dist/deploytool-$os-$arch$ext" ./cmd/deploytool
done
