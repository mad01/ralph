#!/usr/bin/env bash
# build-local-tool.sh - Example build script for a local package
# This can be referenced from config.toml build commands.

set -euo pipefail

PROJECT_DIR="${1:-$HOME/projects/my-tool}"

echo "Building my-tool from ${PROJECT_DIR}..."
cd "${PROJECT_DIR}"
make clean
make build

echo "Build complete."
