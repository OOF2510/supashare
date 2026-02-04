project_name := file_name(justfile_directory())
dist_dir     := "dist"

# Default target
default:
    @just --list

# Clean
# Remove the dist directory
clean:
    echo "=== Cleaning dist directory... ==="
    rm -rf {{dist_dir}}

# Build
# Usage: just build v1
[script]
build version:
    echo "=== Running gofmt, go mod download and go mod tidy... ==="
    gofmt -w .
    go mod download
    go mod tidy

    echo "=== Building the project ({{project_name}}-{{version}})... ==="
    binary_name="{{project_name}}-{{version}}.x86_64"
    binary_dir="{{dist_dir}}/{{version}}"
    binary_path="$binary_dir/$binary_name"

    mkdir -p "$binary_dir"
    go build -v -ldflags='-s -w' -trimpath -o "$binary_path" .

    echo '=== Checking for UPX... ==='
    if command -v upx >/dev/null 2>&1; then
        upx --best --lzma "$binary_path"
    else
        echo 'UPX not found, skipping compression'
    fi

    chmod +x "$binary_path"
    
    echo '=== Copying pages... ==='
    cp -rv pages/ "$binary_dir/pages/"

    if [ -f .env ]; then
        echo '=== Linking .env file... ==='
        ln -sf "$(pwd)/.env" "$binary_dir/.env"
        echo '.env file symlinked'
    fi

    echo '=== Binary size: ==='
    ls -lh "$binary_path"
    echo "=== Build completed ==="