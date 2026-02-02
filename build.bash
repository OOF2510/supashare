#!/bin/bash
set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi
VERSION=${1}

PROJECT_NAME=$(basename "$PWD")
DIR=$(pwd)

echo "=== Running gofmt, go mod download and go mod tidy... ==="
gofmt -w .
go mod download
go mod tidy

echo "=== Building the project... ==="
mkdir -p dist/${VERSION}
go build -v -ldflags="-s -w" -trimpath -o dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64 .

echo "=== Compressing binary with UPX... ==="
if command -v upx &> /dev/null; then
    upx --best --lzma dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64
else
    echo "UPX not found, skipping compression"
fi

echo "=== Build finished. Setting executable permissions... ==="
chmod +x dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64

echo "=== Copying pages... ==="
cp -rv pages/ dist/${VERSION}/pages/

if [ -f .env ]; then
  echo "=== Linking .env file... ==="
  ln -sf ${DIR}/.env ${DIR}/dist/${VERSION}/.env
fi

echo "=== Binary size: ==="
ls -lh ./dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64

echo "=== Build completed ==="