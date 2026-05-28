#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Doctor produces correct summary with mixed pass/fail ==="

VOLUME_NAME="ralph-test-doctor-report-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Set up the test environment:
# - Create valid symlinks for test_bashrc and test_vimrc
# - Create a broken symlink for broken_link
# - Ensure .config dir exists but .missing_test_dir does not
echo "Setting up test environment..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    -v "${TEST_CASE_DIR}/config.toml:/tmp/config.toml:ro" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/tmp/dotfiles_src:ro" \
    ${IMAGE_NAME} -c "
        mkdir -p /home/testuser/.config/ralph
        mkdir -p /home/testuser/dotfiles_src
        cp /tmp/config.toml /home/testuser/.config/ralph/config.toml
        cp /tmp/dotfiles_src/.test_bashrc /home/testuser/dotfiles_src/.test_bashrc
        cp /tmp/dotfiles_src/.test_vimrc /home/testuser/dotfiles_src/.test_vimrc
        # Create valid symlinks
        ln -sf /home/testuser/dotfiles_src/.test_bashrc /home/testuser/.actual_bashrc
        ln -sf /home/testuser/dotfiles_src/.test_vimrc /home/testuser/.actual_vimrc
        # Create broken symlink (source does not exist)
        ln -sf /home/testuser/dotfiles_src/.nonexistent_source /home/testuser/.broken_target
        # .config exists, .missing_test_dir does not
    "

# Run ralph doctor and capture JSON stdout only (expect non-zero exit)
echo ""
echo "Running ralph doctor -o json..."
set +e
JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} doctor -o json 2>/dev/null)
DOCTOR_EXIT=$?
set -e

echo "JSON output:"
echo "${JSON}"
echo ""
echo "Doctor exit code: ${DOCTOR_EXIT}"

# Assert at least one failure
echo "$JSON" | jq -e '.summary.failed >= 1' >/dev/null || {
    echo "ERROR: Expected summary.failed >= 1"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: summary.failed >= 1"

# Assert step "broken_link" with status "fail"
echo "$JSON" | jq -e '[.phases[].steps[]|select(.name=="broken_link" and .status=="fail")]|length>=1' >/dev/null || {
    echo "ERROR: No step named 'broken_link' with status 'fail'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: step broken_link with status fail present"

# Assert step "nonexistent_tool_xyz" with status "warn"
echo "$JSON" | jq -e '[.phases[].steps[]|select(.name=="nonexistent_tool_xyz" and .status=="warn")]|length>=1' >/dev/null || {
    echo "ERROR: No step named 'nonexistent_tool_xyz' with status 'warn'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: step nonexistent_tool_xyz with status warn present"

# Assert JSON exit_code is 1
echo "$JSON" | jq -e '.exit_code == 1' >/dev/null || {
    echo "ERROR: JSON exit_code is not 1"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: JSON exit_code == 1"

# Verify captured exit code is 1 (has failures)
if [ "$DOCTOR_EXIT" -ne 1 ]; then
    echo "ERROR: Expected exit code 1 (has failures), got ${DOCTOR_EXIT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Exit code is 1 (has failures)"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: Doctor report output verified ==="
echo "  - summary.failed >= 1"
echo "  - step broken_link with status fail"
echo "  - step nonexistent_tool_xyz with status warn"
echo "  - JSON exit_code == 1"
echo "  - process exit code 1 for failures"
