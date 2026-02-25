#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: ralph update pulls dotfiles repo and --no-pull skips it ==="

VOLUME_NAME="ralph-test-update-no-pull-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Initialize the dotfiles source as a git repo with a remote
echo "Initializing git repos..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    ${IMAGE_NAME} -c "
        # Create a bare repo to act as the remote
        git init --bare /home/testuser/dotfiles_remote.git
        cd /home/testuser/dotfiles_remote.git
        git config receive.denyCurrentBranch ignore

        # Clone from it to create dotfiles_src with a tracking branch
        git clone /home/testuser/dotfiles_remote.git /home/testuser/dotfiles_src
        cd /home/testuser/dotfiles_src
        git config user.email 'test@test.com'
        git config user.name 'Test'
        cp /tmp/dotfiles_src_init/.keep . 2>/dev/null || true
        echo 'initial' > content.txt
        git add -A
        git commit -m 'initial'
        git push origin master
    " 2>/dev/null

# Copy config
echo "Setting up config..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/.config/ralph
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    "

# Push a new commit to the remote (simulating upstream changes)
echo "Pushing upstream change to remote..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        echo 'updated' > content.txt
        git add -A
        git commit -m 'upstream change'
        git push origin master

        # Reset local back so there is something to pull
        git reset --hard HEAD~1
    " 2>/dev/null

# Verify local is behind remote
echo "Verifying local is behind remote..."
BEHIND_CHECK=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        git fetch origin
        git rev-list --count HEAD..origin/master
    " 2>/dev/null)
echo "Commits behind: ${BEHIND_CHECK}"
if [ "$BEHIND_CHECK" != "1" ]; then
    echo "ERROR: Expected local to be 1 commit behind remote, got ${BEHIND_CHECK}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Test 1: ralph update --no-pull should skip the dotfiles pull
echo ""
echo "=== Test 1: ralph update --no-pull --dry-run -v (should skip pull) ==="
NOPULL_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} update --no-pull --dry-run -v 2>&1)
echo "${NOPULL_OUTPUT}"

if ! echo "${NOPULL_OUTPUT}" | grep -qF 'Dotfiles repo'; then
    echo "ERROR: Output missing 'Dotfiles repo' phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Dotfiles repo phase present"

if ! echo "${NOPULL_OUTPUT}" | grep -q 'skip'; then
    echo "ERROR: Output should show skip for --no-pull"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Skip message present for --no-pull"

# Verify local is still behind (pull was skipped)
STILL_BEHIND=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        git fetch origin
        git rev-list --count HEAD..origin/master
    " 2>/dev/null)
if [ "$STILL_BEHIND" != "1" ]; then
    echo "ERROR: Expected local to still be 1 commit behind after --no-pull, got ${STILL_BEHIND}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Local still behind remote (pull was skipped)"

# Test 2: ralph update (without --no-pull) should pull the dotfiles repo
echo ""
echo "=== Test 2: ralph update -v (should pull dotfiles repo) ==="
PULL_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} update -v 2>&1)
echo "${PULL_OUTPUT}"

if ! echo "${PULL_OUTPUT}" | grep -qF 'Dotfiles repo'; then
    echo "ERROR: Output missing 'Dotfiles repo' phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Dotfiles repo phase present"

if ! echo "${PULL_OUTPUT}" | grep -q 'Pulling dotfiles repo'; then
    echo "ERROR: Output should show 'Pulling dotfiles repo'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Pulling message present"

# Verify local is now up to date
UP_TO_DATE=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        cd /home/testuser/dotfiles_src
        git fetch origin
        git rev-list --count HEAD..origin/master
    " 2>/dev/null)
if [ "$UP_TO_DATE" != "0" ]; then
    echo "ERROR: Expected local to be up to date after pull, got ${UP_TO_DATE} commits behind"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Local is up to date with remote (pull succeeded)"

# Verify content was actually updated
CONTENT=$(docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "cat /home/testuser/dotfiles_src/content.txt")
if [ "$CONTENT" != "updated" ]; then
    echo "ERROR: Expected content.txt to be 'updated' after pull, got '${CONTENT}'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: File content updated by pull"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: ralph update dotfiles pull verified ==="
echo "  - --no-pull: skipped pull, local stayed behind"
echo "  - default: pulled dotfiles repo, local caught up"
