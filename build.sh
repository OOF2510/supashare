#!/bin/bash
set -e

# Get first argument as version
if [ -z "$1" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi
VERSION=${1}

PROJECT_NAME=$(basename "$PWD")

echo "Running gofmt, go mod download and go mod tidy..."
gofmt -w .
go mod download
go mod tidy

echo "Building the project..."
mkdir -p dist
go build -v -x -race -o dist/${PROJECT_NAME}-${VERSION}.x86_64 .
echo "Build completed. Executable is located at dist/${PROJECT_NAME}-${VERSION}.x86_64"

# echo "bulding test"
# cd test/
# gofmt -w .
# go mod download
# go mod tidy
# go build -v -x -o ../dist/test.x86_64 .
# echo "Test is at dist/test.x86_64"
# cd ..
