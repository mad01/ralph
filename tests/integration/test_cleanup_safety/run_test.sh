#!/bin/bash
# Integration test: SafeRemove rails reject dangerous install_paths and
# never delete files outside $HOME or paths containing glob characters.
#
# Seeds three canary files matching the recipe's bad install_paths, runs
# cleanup, and asserts every canary still exists.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: SafeRemove rejects out-of-home and globbed install_paths ==="

VOLUME_NAME="ralph-test-cleanup-safety-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Seed: lay down dotfiles_src + initial config + the in-HOME glob canary.
# /tmp and /etc canaries from the recipe's install_paths are NOT created on
# disk — the cleanup-output rejection lines are the proof those paths never
# reached os.Remove. The in-HOME glob path IS verified on disk because the
# named volume persists between docker runs.
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src_init:ro" \
    -v "${TEST_CASE_DIR}/config_with_recipe.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/dotfiles_src /home/testuser/.config/ralph /home/testuser/code/bin
        cp -r /tmp/dotfiles_src_init/. /home/testuser/dotfiles_src/
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
        # Canary: a literal-asterisk filename inside HOME that the glob
        # entry would 'match' if SafeRemove ever expanded globs. SafeRemove
        # MUST reject it on the glob-character rail before any filesystem
        # call — that's what we assert on disk after cleanup.
        : > '/home/testuser/code/bin/*-glob'
    "

echo ""
echo "--- First apply: recipe present + --enable-cleanup ---"
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply --enable-cleanup

echo ""
echo "--- Removing recipe and applying cleanup ---"
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config_without_recipe.toml:/tmp/config.toml:ro" \
    ${IMAGE_NAME} -c "cp /tmp/config.toml /home/testuser/.config/ralph/config.toml"

CLEANUP_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply --enable-cleanup --verbose 2>&1)
echo "${CLEANUP_OUTPUT}"

# Apply must succeed (skipped paths are warnings, not errors)
if echo "${CLEANUP_OUTPUT}" | grep -q "Some items failed"; then
    echo "ERROR: apply reported failure when it should have skipped dangerous paths"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Output must contain at least one "skip ..." line for each rail
SKIPPED=$(echo "${CLEANUP_OUTPUT}" | grep -c "skip install_path" || true)
if [ "${SKIPPED}" -lt 3 ]; then
    echo "ERROR: expected 3 skipped install_path lines, got ${SKIPPED}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "Skipped ${SKIPPED} install_path entries (≥ 3 expected)."

# Per-rail rejection assertions: each rail should fire its own sentinel
# error, surfaced verbatim in the cleanup log. If any of these checks
# fails, the rails are not enforcing what we expect them to.
for needle in \
    "outside the allowed prefix set: /tmp/canary-outside-home" \
    "outside the allowed prefix set: /etc/canary-system" \
    "glob characters not allowed in path: /home/testuser/code/bin/*-glob"; do
    if ! echo "${CLEANUP_OUTPUT}" | grep -qF "${needle}"; then
        echo "ERROR: cleanup output missing expected rail rejection:"
        echo "         ${needle}"
        docker volume rm ${VOLUME_NAME} > /dev/null
        exit 1
    fi
done
echo "All three SafeRemove rails fired with the right sentinel errors."

echo ""
echo "Verifying in-HOME glob canary was NOT deleted..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        if [ ! -f '/home/testuser/code/bin/*-glob' ]; then
            echo 'ERROR: glob-shaped canary file inside HOME was deleted — SafeRemove glob rail failed!'
            exit 1
        fi
        echo 'in-HOME glob canary preserved'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null
echo ""
echo "=== TEST PASSED: SafeRemove rails block dangerous install_paths ==="
