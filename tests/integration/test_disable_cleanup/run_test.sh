#!/bin/bash
# Integration test: the declarative uninstall flow that replaces `ralph down`.
#
# Flow:
#   1. up --no-sync --enable-cleanup installs the demo recipe (symlink + state).
#   2. Verify the symlink and recipe state exist.
#   3. `ralph disable demo` writes an enable=false override to config.toml.
#   4. up --no-sync --enable-cleanup reconciles: the now-disabled recipe's
#      artifacts become orphans and are removed by the cleanup phase.
#   5. Verify the JSON reports a clean run with a Cleanup phase, and the symlink
#      is gone from the volume.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: disable + up --enable-cleanup uninstalls a recipe ==="

VOLUME_NAME="ralph-test-disable-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Lay down dotfiles_src (with the demo recipe) and a writable config.
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/dotfiles_src /home/testuser/.config/ralph
        cp -r /tmp/dotfiles_src_init/. /home/testuser/dotfiles_src/
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    "

echo ""
echo "--- up --no-sync --enable-cleanup: install the demo recipe ---"
UP_JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync --enable-cleanup -o json 2>/dev/null)
echo "${UP_JSON}"

echo "$UP_JSON" | jq -e '.summary.failed == 0' >/dev/null || {
    echo "ERROR: install (up) reported failures"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: up succeeded with no failures"

echo ""
echo "Verifying install artifacts..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        [ -L /home/testuser/.demo_link ] || { echo 'ERROR: symlink missing after install'; exit 1; }
        grep -q demo /home/testuser/.config/ralph/.recipe_state || { echo 'ERROR: recipe not in manifest after install'; cat /home/testuser/.config/ralph/.recipe_state; exit 1; }
        echo 'install OK: symlink + manifest entry present'
    "

echo ""
echo "--- ralph disable demo: write enable=false override ---"
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} disable demo

docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        grep -q 'enable = false' /home/testuser/.config/ralph/config.toml || {
            echo 'ERROR: disable did not write enable=false to config.toml'
            cat /home/testuser/.config/ralph/config.toml
            exit 1
        }
        echo 'disable OK: enable=false override present in config.toml'
    "

echo ""
echo "--- up --no-sync --enable-cleanup: reconcile (remove orphaned artifacts) ---"
set +e
RECONCILE_JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync --enable-cleanup -o json 2>/dev/null)
RECONCILE_EXIT=$?
set -e
echo "${RECONCILE_JSON}"
echo "reconcile exit code: ${RECONCILE_EXIT}"

echo "$RECONCILE_JSON" | jq -e '.command == "up"' >/dev/null || {
    echo "ERROR: reconcile output is not an 'up' report (invalid/missing JSON)"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: reconcile emitted a valid JSON report"

echo "$RECONCILE_JSON" | jq -e '.exit_code == 0' >/dev/null || {
    echo "ERROR: reconcile reported a non-zero exit_code"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: reconcile exit_code is 0"

echo "$RECONCILE_JSON" | jq -e '[.phases[]|select(.name=="Cleanup")]|length>=1' >/dev/null || {
    echo "ERROR: reconcile output missing Cleanup phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: Cleanup phase present in reconcile output"

if [ "$RECONCILE_EXIT" -ne 0 ]; then
    echo "ERROR: expected reconcile exit code 0, got ${RECONCILE_EXIT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: reconcile process exit code is 0"

echo ""
echo "Verifying artifacts were removed..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        if [ -L /home/testuser/.demo_link ] || [ -e /home/testuser/.demo_link ]; then
            echo 'ERROR: symlink still present after cleanup'; exit 1
        fi
        echo 'uninstall OK: symlink removed'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: disable + up --enable-cleanup uninstalls a recipe ==="
echo "  - up installed the demo recipe (symlink + state)"
echo "  - disable wrote an enable=false override to config.toml"
echo "  - up --enable-cleanup removed the orphaned symlink via the Cleanup phase"
