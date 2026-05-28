#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status.

# Get the absolute path to the project root (assuming this script is in tests/integration/test_apply_basic)
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "Running integration test for basic apply..."

# Use a named volume to persist /home/testuser between container runs
VOLUME_NAME="ralph-test-home-$(date +%s)" # Unique volume name for this test run
docker volume create ${VOLUME_NAME} > /dev/null

# Run ralph up --no-sync with JSON output and capture stdout only
echo "Running ralph up --no-sync with persistent volume ${VOLUME_NAME}..."
JSON=$(docker run --rm \
    -v "${TEST_CASE_DIR}/config.toml:/home/testuser/.config/ralph/config.toml:ro" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/home/testuser/dotfiles_src:ro" \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync -o json 2>/dev/null)
echo "${JSON}"

# Assert no failures
echo "$JSON" | jq -e '.summary.failed == 0' >/dev/null || {
    echo "ERROR: summary.failed is not 0"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: summary.failed == 0"

# Assert a Dotfiles step named "test_bashrc" with status "ok"
echo "$JSON" | jq -e '[.phases[].steps[]|select(.name=="test_bashrc" and .status=="ok")]|length==1' >/dev/null || {
    echo "ERROR: No Dotfiles step named 'test_bashrc' with status 'ok'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: step test_bashrc with status ok present"

# --- Verification ---
echo "Verifying results in volume ${VOLUME_NAME}..."

VERIFICATION_SCRIPT="
set -e
ls -la /home/testuser # Optional: for debugging

# Check .actual_bashrc symlink
if [ ! -L /home/testuser/.actual_bashrc ]; then
    echo 'ERROR: /home/testuser/.actual_bashrc is not a symlink!'
    exit 1
fi
LINK_TARGET_BASHRC=\$(readlink /home/testuser/.actual_bashrc)
EXPECTED_TARGET_BASHRC='/home/testuser/dotfiles_src/.test_bashrc'
if [ \"\${LINK_TARGET_BASHRC}\" != \"\${EXPECTED_TARGET_BASHRC}\" ]; then
    echo \"ERROR: /home/testuser/.actual_bashrc points to \${LINK_TARGET_BASHRC}, expected \${EXPECTED_TARGET_BASHRC}\"
    exit 1
fi
echo '.actual_bashrc symlink OK'

# Check .actual_vimrc symlink
if [ ! -L /home/testuser/.actual_vimrc ]; then
    echo 'ERROR: /home/testuser/.actual_vimrc is not a symlink!'
    exit 1
fi
LINK_TARGET_VIMRC=\$(readlink /home/testuser/.actual_vimrc)
EXPECTED_TARGET_VIMRC='/home/testuser/dotfiles_src/.test_vimrc'
if [ \"\${LINK_TARGET_VIMRC}\" != \"\${EXPECTED_TARGET_VIMRC}\" ]; then
    echo \"ERROR: /home/testuser/.actual_vimrc points to \${LINK_TARGET_VIMRC}, expected \${EXPECTED_TARGET_VIMRC}\"
    exit 1
fi
echo '.actual_vimrc symlink OK'

# Check content of linked .actual_bashrc
if ! grep -q 'TEST_VAR=\"bashrc_loaded\"' /home/testuser/.actual_bashrc; then
    echo 'ERROR: Content check failed for .actual_bashrc'
    exit 1
fi
echo 'Content of .actual_bashrc OK'

# Check content of linked .actual_vimrc
if ! grep -q 'set number' /home/testuser/.actual_vimrc; then
    echo 'ERROR: Content check failed for .actual_vimrc'
    exit 1
fi
echo 'Content of .actual_vimrc OK'

echo 'Verification successful!'
"

# Run the verification script in a new container with the same named volume
# Override the entrypoint to use /bin/sh directly
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/home/testuser/dotfiles_src:ro" \
    ${IMAGE_NAME} -c "${VERIFICATION_SCRIPT}"

# Clean up the volume
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo "Integration test completed successfully!"
