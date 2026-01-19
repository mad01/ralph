#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="dotter-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Build hooks with run=once are idempotent ==="

# Use a named volume to persist /home/testuser between container runs
VOLUME_NAME="dotter-test-builds-once-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Initialize the dotfiles source with git (required for build state tracking)
echo "Initializing git repo in dotfiles_src..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/dotfiles_src
        cp -r /tmp/dotfiles_src_init/* /home/testuser/dotfiles_src/ 2>/dev/null || true
        cd /home/testuser/dotfiles_src
        git init
        git config user.email 'test@test.com'
        git config user.name 'Test'
        git add -A
        git commit -m 'initial' || true
    " 2>/dev/null

# Copy config and build script
echo "Setting up test files..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    -v "${TEST_CASE_DIR}/build_script.sh:/tmp/build_script.sh:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/.config/dotter
        cp /tmp/config.toml /home/testuser/.config/dotter/config.toml
        cp /tmp/build_script.sh /home/testuser/dotfiles_src/build_script.sh
        chmod +x /home/testuser/dotfiles_src/build_script.sh
        cd /home/testuser/dotfiles_src
        git add -A
        git commit -m 'add build script' || true
    "

# First run: build should execute
echo ""
echo "=== First dotter apply (build should RUN) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply

# Check counter after first run
echo ""
echo "Checking build counter after first run..."
FIRST_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after first run: ${FIRST_COUNT}"

if [ "$FIRST_COUNT" != "1" ]; then
    echo "ERROR: Expected build count to be 1 after first run, got ${FIRST_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Second run: build should be SKIPPED
echo ""
echo "=== Second dotter apply (build should be SKIPPED) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply

# Check counter after second run
echo ""
echo "Checking build counter after second run..."
SECOND_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after second run: ${SECOND_COUNT}"

if [ "$SECOND_COUNT" != "1" ]; then
    echo "ERROR: Expected build count to remain 1 after second run (build should be skipped), got ${SECOND_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Third run with --force: build should run again
echo ""
echo "=== Third dotter apply with --force (build should RUN) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply --force

# Check counter after third run
echo ""
echo "Checking build counter after force run..."
THIRD_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after force run: ${THIRD_COUNT}"

if [ "$THIRD_COUNT" != "2" ]; then
    echo "ERROR: Expected build count to be 2 after force run, got ${THIRD_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: Build hooks idempotency verified ==="
echo "  - First run: build executed (count went from 0 to 1)"
echo "  - Second run: build skipped (count stayed at 1)"
echo "  - Force run: build executed (count went from 1 to 2)"
