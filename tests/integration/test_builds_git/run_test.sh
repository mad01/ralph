#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="dotter-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Build hooks re-run when git changes occur ==="

# Use a named volume to persist /home/testuser between container runs
VOLUME_NAME="dotter-test-builds-git-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Initialize the dotfiles source with git
echo "Initializing git repo in dotfiles_src..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/dotfiles_src
        cd /home/testuser/dotfiles_src
        git init
        git config user.email 'test@test.com'
        git config user.name 'Test'
        echo 'initial' > test.txt
        git add -A
        git commit -m 'initial'
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
        git commit -m 'add build script'
    "

# First run: build should execute
echo ""
echo "=== First dotter apply (build should RUN) ==="
FIRST_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply 2>&1)
echo "${FIRST_OUTPUT}"

# Verify summary contains Builds phase
if ! echo "${FIRST_OUTPUT}" | grep -qF -- '--- Summary ---'; then
    echo "ERROR: First apply output does not contain '--- Summary ---'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "Summary section present in first apply output"

if ! echo "${FIRST_OUTPUT}" | grep -q 'Builds:'; then
    echo "ERROR: First apply output does not contain Builds phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "Builds phase present in first apply output"

FIRST_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after first run: ${FIRST_COUNT}"

if [ "$FIRST_COUNT" != "1" ]; then
    echo "ERROR: Expected build count to be 1, got ${FIRST_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Second run without changes: build should be SKIPPED
echo ""
echo "=== Second dotter apply (no changes, build should be SKIPPED) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply

SECOND_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after second run: ${SECOND_COUNT}"

if [ "$SECOND_COUNT" != "1" ]; then
    echo "ERROR: Expected build count to remain 1 (no changes), got ${SECOND_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Make a new git commit to change the hash
echo ""
echo "=== Making new git commit to change hash ==="
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        echo 'new content' >> test.txt
        git add -A
        git commit -m 'new commit'
    "

# Third run with new commit: build should re-run
echo ""
echo "=== Third dotter apply (git hash changed, build should RUN) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply

THIRD_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after third run: ${THIRD_COUNT}"

if [ "$THIRD_COUNT" != "2" ]; then
    echo "ERROR: Expected build count to be 2 after git change, got ${THIRD_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Test uncommitted changes detection
echo ""
echo "=== Adding uncommitted changes ==="
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        echo 'uncommitted change' >> test.txt
    "

# Fourth run with uncommitted changes: build should re-run
echo ""
echo "=== Fourth dotter apply (uncommitted changes, build should RUN) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply

FOURTH_COUNT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_counter 2>/dev/null || echo 0")

echo "Build count after fourth run: ${FOURTH_COUNT}"

if [ "$FOURTH_COUNT" != "3" ]; then
    echo "ERROR: Expected build count to be 3 after uncommitted changes, got ${FOURTH_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: Build hooks git change detection verified ==="
echo "  - First run: build executed (count = 1)"
echo "  - Second run: build skipped (no changes, count = 1)"
echo "  - Third run: build executed (git hash changed, count = 2)"
echo "  - Fourth run: build executed (uncommitted changes, count = 3)"
