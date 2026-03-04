#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: ralph sync only pulls, does not build ==="

VOLUME_NAME="ralph-test-sync-packages-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Set up git repos and config
echo "Setting up git repos and config..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        # Create dotfiles_src as a git repo
        mkdir -p /home/testuser/dotfiles_src
        cd /home/testuser/dotfiles_src
        git init
        git config user.email 'test@test.com'
        git config user.name 'Test'
        echo 'dotfiles' > README.md
        git add -A
        git commit -m 'initial'

        # Create a bare repo for the remote package
        git init --bare /home/testuser/remote_bare.git
        cd /home/testuser/remote_bare.git
        git config receive.denyCurrentBranch ignore

        # Clone from bare, add content, push
        git clone /home/testuser/remote_bare.git /tmp/remote_src
        cd /tmp/remote_src
        git config user.email 'test@test.com'
        git config user.name 'Test'
        echo 'v1' > version.txt
        git add -A
        git commit -m 'initial remote'
        git push origin master

        # Copy config
        mkdir -p /home/testuser/.config/ralph
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    " 2>/dev/null

# Test 1: ralph sync clones remote, no build output
echo ""
echo "=== Test 1: ralph sync (should clone, NO build) ==="
SYNC_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} sync -v 2>&1)
echo "${SYNC_OUTPUT}"

if ! echo "${SYNC_OUTPUT}" | grep -q 'cloning'; then
    echo "ERROR: Expected 'cloning' in sync output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package cloned"

# Verify no build log was created
BUILD_LOG=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")
if [ "$BUILD_LOG" != "no log" ]; then
    echo "ERROR: Sync should NOT create build log, but got: ${BUILD_LOG}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: No build output from sync"

# Verify clone directory exists
CLONE_EXISTS=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "test -d /home/testuser/pkg/test_remote_pkg/.git && echo 'yes' || echo 'no'")
if [ "$CLONE_EXISTS" != "yes" ]; then
    echo "ERROR: Expected clone directory to exist"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Clone directory exists"

# Test 2: Push upstream change, ralph sync pulls it, no build
echo ""
echo "=== Test 2: Push upstream change, ralph sync (should pull, NO build) ==="
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        git clone /home/testuser/remote_bare.git /tmp/remote_update
        cd /tmp/remote_update
        git config user.email 'test@test.com'
        git config user.name 'Test'
        echo 'v2' > version.txt
        git add -A
        git commit -m 'upstream update'
        git push origin master
    " 2>/dev/null

SYNC2_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} sync -v 2>&1)
echo "${SYNC2_OUTPUT}"

if ! echo "${SYNC2_OUTPUT}" | grep -q 'pulling'; then
    echo "ERROR: Expected 'pulling' in sync output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package pulled"

# Verify content was updated
CONTENT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/pkg/test_remote_pkg/version.txt")
if [ "$CONTENT" != "v2" ]; then
    echo "ERROR: Expected version.txt to be 'v2' after pull, got '${CONTENT}'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: File content updated by pull"

# Verify still no build log
BUILD_LOG2=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")
if [ "$BUILD_LOG2" != "no log" ]; then
    echo "ERROR: Sync should NOT create build log after pull, but got: ${BUILD_LOG2}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: No build output from sync after pull"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: ralph sync only pulls, does not build ==="
echo "  - sync clones remote package, no builds"
echo "  - sync pulls upstream changes, no builds"
