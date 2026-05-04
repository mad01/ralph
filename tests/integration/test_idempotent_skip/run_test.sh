#!/bin/bash
# Integration test: idempotent = true skips a build whose content hash
# matches the last successful run, and re-runs it when the command list
# changes.
#
#   1. First apply  -> counter goes 0 -> 1 (build runs)
#   2. Second apply -> counter stays 1 (skipped: content unchanged)
#   3. Edit config to add a second command -> counter goes 1 -> 2 (hash changed)
#   4. Fourth apply -> counter stays 2 (skipped at new hash)
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: idempotent build skips when content hash matches ==="

VOLUME_NAME="ralph-test-idempotent-skip-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Seed dotfiles_src with the bump_counter script and lay down the v1 config.
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/bump_counter.sh:/tmp/bump_counter.sh:ro" \
    -v "${TEST_CASE_DIR}/config_idempotent.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/dotfiles_src /home/testuser/.config/ralph
        cp /tmp/bump_counter.sh /home/testuser/dotfiles_src/bump_counter.sh
        chmod +x /home/testuser/dotfiles_src/bump_counter.sh
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    "

read_counter() {
    docker run --rm \
        --entrypoint /bin/sh \
        -v "${VOLUME_NAME}:/home/testuser" \
        ${IMAGE_NAME} -c "cat /home/testuser/.idem_counter 2>/dev/null || echo 0"
}

assert_counter() {
    local expected="$1"
    local label="$2"
    local got
    got=$(read_counter)
    if [ "$got" != "$expected" ]; then
        echo "ERROR: ${label}: expected counter=${expected}, got ${got}"
        docker volume rm ${VOLUME_NAME} > /dev/null
        exit 1
    fi
    echo "${label}: counter=${got} ✓"
}

echo ""
echo "--- Apply #1 (build should RUN) ---"
docker run --rm -v "${VOLUME_NAME}:/home/testuser" ${IMAGE_NAME} apply
assert_counter 1 "after apply #1"

echo ""
echo "--- Apply #2 (build should SKIP, content unchanged) ---"
SECOND_OUTPUT=$(docker run --rm -v "${VOLUME_NAME}:/home/testuser" ${IMAGE_NAME} apply --verbose 2>&1)
echo "${SECOND_OUTPUT}"
if ! echo "${SECOND_OUTPUT}" | grep -q "content unchanged"; then
    echo "ERROR: apply #2 did not log 'content unchanged' for the idempotent build"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
assert_counter 1 "after apply #2"

echo ""
echo "--- Editing config to change command list (hash should change) ---"
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config_idempotent_v2.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "cp /tmp/config.toml /home/testuser/.config/ralph/config.toml"

echo ""
echo "--- Apply #3 (build should RUN, hash changed) ---"
docker run --rm -v "${VOLUME_NAME}:/home/testuser" ${IMAGE_NAME} apply
assert_counter 2 "after apply #3"

echo ""
echo "--- Apply #4 (build should SKIP at new hash) ---"
docker run --rm -v "${VOLUME_NAME}:/home/testuser" ${IMAGE_NAME} apply
assert_counter 2 "after apply #4"

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: idempotent skip honors content hash ==="
