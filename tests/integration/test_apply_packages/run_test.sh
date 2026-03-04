#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: ralph apply builds packages after sync ==="

VOLUME_NAME="ralph-test-apply-packages-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Set up git repos and config
echo "Setting up git repos and config..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        # Create dotfiles_src as a git repo with local_pkg subdir
        mkdir -p /home/testuser/dotfiles_src/local_pkg
        cd /home/testuser/dotfiles_src
        cp /tmp/dotfiles_src_init/local_pkg/build.sh local_pkg/build.sh
        git init
        git config user.email 'test@test.com'
        git config user.name 'Test'
        git add -A
        git commit -m 'initial'

        # Create a bare repo for the remote package
        git init --bare /home/testuser/remote_bare.git
        cd /home/testuser/remote_bare.git
        git config receive.denyCurrentBranch ignore

        # Clone from bare to create a real remote repo, then push
        git clone /home/testuser/remote_bare.git /tmp/remote_src
        cd /tmp/remote_src
        git config user.email 'test@test.com'
        git config user.name 'Test'
        echo 'remote content' > README.md
        git add -A
        git commit -m 'initial remote'
        git push origin master

        # Copy config
        mkdir -p /home/testuser/.config/ralph
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    " 2>/dev/null

# Test 1: ralph sync clones remote package
echo ""
echo "=== Test 1: ralph sync (should clone remote package) ==="
SYNC_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} sync -v 2>&1)
echo "${SYNC_OUTPUT}"

if ! echo "${SYNC_OUTPUT}" | grep -q 'cloning'; then
    echo "ERROR: Expected 'cloning' in sync output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package cloned by sync"

# Verify no build output from sync
if echo "${SYNC_OUTPUT}" | grep -q '\[build'; then
    echo "ERROR: Sync should NOT run builds"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Sync did not run builds"

# Test 2: ralph apply builds both packages
echo ""
echo "=== Test 2: ralph apply (should build both packages) ==="
APPLY_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply -v 2>&1)
echo "${APPLY_OUTPUT}"

BUILD_LOG=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")
echo "Build log: ${BUILD_LOG}"

if ! echo "${BUILD_LOG}" | grep -q 'local built'; then
    echo "ERROR: Expected 'local built' in build log"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Local package was built"

if ! echo "${BUILD_LOG}" | grep -q 'remote built'; then
    echo "ERROR: Expected 'remote built' in build log"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package was built"

# Test 3: ralph apply again should skip (up to date)
echo ""
echo "=== Test 3: ralph apply again (should skip, up to date) ==="
# Clear the build log to verify no new builds happen
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "rm -f /home/testuser/.build_log"

APPLY2_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply -v 2>&1)
echo "${APPLY2_OUTPUT}"

BUILD_LOG2=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")

if [ "$BUILD_LOG2" != "no log" ]; then
    echo "ERROR: Expected no builds on second apply, but build log exists: ${BUILD_LOG2}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: No builds on second apply (up to date)"

# Test 4: Make a change in local package, apply should rebuild only it
echo ""
echo "=== Test 4: Change local package, ralph apply (should rebuild only changed) ==="
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src/local_pkg
        echo 'change' >> build.sh
        git add -A
        git commit -m 'local change'
    " 2>/dev/null

APPLY3_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply -v 2>&1)
echo "${APPLY3_OUTPUT}"

BUILD_LOG3=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")
echo "Build log: ${BUILD_LOG3}"

if ! echo "${BUILD_LOG3}" | grep -q 'local built'; then
    echo "ERROR: Expected 'local built' after local change"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Local package rebuilt after change"

LOCAL_COUNT=$(echo "${BUILD_LOG3}" | grep -c 'local built' || true)
REMOTE_COUNT=$(echo "${BUILD_LOG3}" | grep -c 'remote built' || true)

if [ "$REMOTE_COUNT" != "0" ]; then
    echo "ERROR: Remote package should NOT have been rebuilt"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package was NOT rebuilt (unchanged)"

# Test 5: ralph apply --force rebuilds all
echo ""
echo "=== Test 5: ralph apply --force (should rebuild all) ==="
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "rm -f /home/testuser/.build_log"

FORCE_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply --force -v 2>&1)
echo "${FORCE_OUTPUT}"

BUILD_LOG4=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/.build_log 2>/dev/null || echo 'no log'")
echo "Build log: ${BUILD_LOG4}"

if ! echo "${BUILD_LOG4}" | grep -q 'local built'; then
    echo "ERROR: Expected 'local built' in force rebuild"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Local package rebuilt with --force"

if ! echo "${BUILD_LOG4}" | grep -q 'remote built'; then
    echo "ERROR: Expected 'remote built' in force rebuild"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Remote package rebuilt with --force"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: ralph apply builds packages correctly ==="
echo "  - sync clones remote, no builds"
echo "  - apply builds both packages"
echo "  - second apply skips (up to date)"
echo "  - change triggers selective rebuild"
echo "  - --force rebuilds all"
