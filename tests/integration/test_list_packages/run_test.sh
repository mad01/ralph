#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: ralph list packages with update status ==="

VOLUME_NAME="ralph-test-list-packages-$(date +%s)"
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

# Test 1: ralph list should show both packages with "never built" / "not cloned"
echo ""
echo "=== Test 1: ralph list (before any updates) ==="
LIST_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} list 2>&1)
echo "${LIST_OUTPUT}"

if ! echo "${LIST_OUTPUT}" | grep -qF 'Managed Packages:'; then
    echo "ERROR: Missing 'Managed Packages:' header"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Managed Packages header present"

if ! echo "${LIST_OUTPUT}" | grep -qF 'test_local_pkg'; then
    echo "ERROR: Missing test_local_pkg in output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_local_pkg present"

if ! echo "${LIST_OUTPUT}" | grep -qF 'test_remote_pkg'; then
    echo "ERROR: Missing test_remote_pkg in output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_remote_pkg present"

if ! echo "${LIST_OUTPUT}" | grep -q 'never built\|not cloned'; then
    echo "ERROR: Expected 'never built' or 'not cloned' status"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Needs-update status present"

# Test 2: ralph list --source local should only show local package
echo ""
echo "=== Test 2: ralph list --source local ==="
LOCAL_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} list --source local 2>&1)
echo "${LOCAL_OUTPUT}"

if ! echo "${LOCAL_OUTPUT}" | grep -qF 'test_local_pkg'; then
    echo "ERROR: Missing test_local_pkg in --source local output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_local_pkg present in --source local"

if echo "${LOCAL_OUTPUT}" | grep -qF 'test_remote_pkg'; then
    echo "ERROR: test_remote_pkg should NOT appear in --source local output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_remote_pkg filtered out in --source local"

# Test 3: ralph list --source remote should only show remote package
echo ""
echo "=== Test 3: ralph list --source remote ==="
REMOTE_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} list --source remote 2>&1)
echo "${REMOTE_OUTPUT}"

if ! echo "${REMOTE_OUTPUT}" | grep -qF 'test_remote_pkg'; then
    echo "ERROR: Missing test_remote_pkg in --source remote output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_remote_pkg present in --source remote"

if echo "${REMOTE_OUTPUT}" | grep -qF 'test_local_pkg'; then
    echo "ERROR: test_local_pkg should NOT appear in --source remote output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: test_local_pkg filtered out in --source remote"

# Test 4: run ralph sync then ralph apply, then verify list shows "Up to date"
echo ""
echo "=== Test 4: ralph sync + ralph apply then ralph list (should show up to date) ==="
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} sync 2>&1
echo "Sync complete."
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply 2>&1
echo "Apply complete."

AFTER_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} list 2>&1)
echo "${AFTER_OUTPUT}"

if ! echo "${AFTER_OUTPUT}" | grep -q 'Up to date'; then
    echo "ERROR: Expected 'Up to date' status after sync+apply"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Up to date status present after sync+apply"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: ralph list packages with update status ==="
echo "  - list: shows both packages with status"
echo "  - --source local: filters to local only"
echo "  - --source remote: filters to remote only"
echo "  - after sync+apply: shows up-to-date status"
