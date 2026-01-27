#!/bin/bash
set -e

# Get first argument as version
if [ -z "$1" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi
VERSION=${1}

PROJECT_NAME=$(basename "$PWD")
DIR=$(pwd)

DOTENV_EXISTS=$(ls .env 2>/dev/null || true)

echo "=== Running gofmt, go mod download and go mod tidy... ==="
gofmt -w .
go mod download
go mod tidy

echo "=== Building the project... ==="
mkdir -p dist/${VERSION}
go build -v -x -race -o dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64 main.go
chmod +x dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64

echo "=== Copying pages... ==="
cp -rv pages/ dist/${VERSION}/pages/
echo "=== Pages copied to dist/${VERSION}/pages/ ==="
if [ -n "$DOTENV_EXISTS" ]; then
  echo "=== .env file found. ==="
  echo "=== Lnking .env file... ==="
  ln -sf ${DIR}/.env ${DIR}/dist/${VERSION}/.env
  echo "=== .env file symlinked to dist/${VERSION}/.env ==="
fi

echo "=== Build completed. Executable is located at dist/${VERSION}/${PROJECT_NAME}-${VERSION}.x86_64 ==="

# echo "bulding test"
# cd test/
# gofmt -w .
# go mod download
# go mod tidy
# go build -v -x -o ../dist/test.x86_64 .
# echo "Test is at dist/test.x86_64"
# cd ..
