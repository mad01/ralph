#!/bin/bash
# Integration test: --enable-cleanup removes orphaned artifacts when a recipe
# is removed from the config.
#
# Flow:
#   1. Apply with the recipe present + --enable-cleanup. Record state.
#   2. Swap the config to one that omits the recipe.
#   3. Apply again with --enable-cleanup. Cleanup phase runs.
#   4. Verify symlink + install_path are gone, manifest no longer mentions the recipe.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: cleanup with delete_behavior=delete (default) ==="

VOLUME_NAME="ralph-test-cleanup-delete-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Lay down dotfiles_src and the initial config
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    -v "${TEST_CASE_DIR}/config_with_recipe.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/dotfiles_src /home/testuser/.config/ralph
        cp -r /tmp/dotfiles_src_init/. /home/testuser/dotfiles_src/
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
    "

echo ""
echo "--- First up --no-sync: recipe present + --enable-cleanup ---"
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync --enable-cleanup

echo ""
echo "Verifying first-apply artifacts..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        [ -L /home/testuser/.test_link ] || { echo 'ERROR: symlink missing after first apply'; exit 1; }
        [ -x /home/testuser/code/bin/test-cleanup-bin ] || { echo 'ERROR: install_path binary missing after first apply'; exit 1; }
        grep -q test_cleanup /home/testuser/.config/ralph/.recipe_state || { echo 'ERROR: recipe not in manifest after first apply'; cat /home/testuser/.config/ralph/.recipe_state; exit 1; }
        echo 'first-apply OK: symlink + binary + manifest entry all present'
    "

# Swap to the config without the recipe and apply with cleanup
echo ""
echo "--- Removing recipe from config and re-applying ---"
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config_without_recipe.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "cp /tmp/config.toml /home/testuser/.config/ralph/config.toml"

CLEANUP_JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync --enable-cleanup -o json 2>/dev/null)
echo "${CLEANUP_JSON}"

# Assert a Cleanup phase exists
echo "$CLEANUP_JSON" | jq -e '[.phases[]|select(.name=="Cleanup")]|length>=1' >/dev/null || {
    echo "ERROR: Cleanup phase not present in output"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: Cleanup phase present"

echo ""
echo "Verifying orphans were removed..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        if [ -L /home/testuser/.test_link ] || [ -e /home/testuser/.test_link ]; then
            echo 'ERROR: symlink still present after cleanup'; exit 1
        fi
        if [ -e /home/testuser/code/bin/test-cleanup-bin ]; then
            echo 'ERROR: install_path binary still present after cleanup'; exit 1
        fi
        if grep -q test_cleanup /home/testuser/.config/ralph/.recipe_state; then
            echo 'ERROR: recipe still in manifest after cleanup'; cat /home/testuser/.config/ralph/.recipe_state; exit 1
        fi
        echo 'cleanup OK: symlink + binary + manifest entry all removed'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: cleanup-delete removes orphans ==="
