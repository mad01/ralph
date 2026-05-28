#!/bin/bash
# Integration test: `ralph down <recipe>` uninstalls a recipe's artifacts.
#
# Flow:
#   1. up --no-sync --enable-cleanup installs the demo recipe (symlink + state).
#   2. Verify the symlink and recipe state exist.
#   3. down demo -y -o json removes the artifacts.
#   4. Verify the JSON reports a clean run with a Cleanup phase, and the symlink
#      is gone from the volume.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: ralph down uninstalls a recipe ==="

VOLUME_NAME="ralph-test-down-$(date +%s)"
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
echo "--- down demo -y: uninstall the recipe ---"
set +e
DOWN_JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} down demo -y -o json 2>/dev/null)
DOWN_EXIT=$?
set -e
echo "${DOWN_JSON}"
echo "down exit code: ${DOWN_EXIT}"

# Validate the down report.
echo "$DOWN_JSON" | jq -e '.command == "down"' >/dev/null || {
    echo "ERROR: down output is not a 'down' report (invalid/missing JSON)"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: down emitted a valid JSON report"

echo "$DOWN_JSON" | jq -e '.exit_code == 0' >/dev/null || {
    echo "ERROR: down reported a non-zero exit_code"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: down exit_code is 0"

echo "$DOWN_JSON" | jq -e '[.phases[]|select(.name=="Cleanup")]|length>=1' >/dev/null || {
    echo "ERROR: down output missing Cleanup phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: Cleanup phase present in down output"

if [ "$DOWN_EXIT" -ne 0 ]; then
    echo "ERROR: expected down exit code 0, got ${DOWN_EXIT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: down process exit code is 0"

echo ""
echo "Verifying artifacts were removed..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        if [ -L /home/testuser/.demo_link ] || [ -e /home/testuser/.demo_link ]; then
            echo 'ERROR: symlink still present after down'; exit 1
        fi
        echo 'uninstall OK: symlink removed'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: ralph down uninstalls a recipe ==="
echo "  - up installed the demo recipe (symlink + state)"
echo "  - down removed the symlink and reported a clean Cleanup phase"
