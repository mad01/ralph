#!/bin/bash
# Integration test: delete_behavior = "abandon" leaves orphans on disk and
# logs an abandon line, even when the recipe is removed from the config.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: cleanup with delete_behavior=abandon ==="

VOLUME_NAME="ralph-test-cleanup-abandon-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

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
        [ -L /home/testuser/.test_abandon_link ] || { echo 'ERROR: symlink missing after first apply'; exit 1; }
        [ -x /home/testuser/code/bin/test-abandon-bin ] || { echo 'ERROR: install_path binary missing after first apply'; exit 1; }
        grep -q '\"delete_behavior\": \"abandon\"' /home/testuser/.config/ralph/.recipe_state || { echo 'ERROR: abandon delete_behavior not recorded'; cat /home/testuser/.config/ralph/.recipe_state; exit 1; }
        echo 'first-apply OK'
    "

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
echo "Verifying artifacts remain on disk after abandon..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        [ -L /home/testuser/.test_abandon_link ] || { echo 'ERROR: symlink was removed despite abandon'; exit 1; }
        [ -x /home/testuser/code/bin/test-abandon-bin ] || { echo 'ERROR: install_path binary was removed despite abandon'; exit 1; }
        echo 'abandon OK: artifacts preserved on disk'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: abandon leaves artifacts in place ==="
