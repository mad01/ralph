#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="dotter-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Doctor produces correct summary with mixed pass/fail ==="

VOLUME_NAME="dotter-test-doctor-report-$(date +%s)"
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
        mkdir -p /home/testuser/.config/dotter
        mkdir -p /home/testuser/dotfiles_src
        cp /tmp/config.toml /home/testuser/.config/dotter/config.toml
        cp /tmp/dotfiles_src/.test_bashrc /home/testuser/dotfiles_src/.test_bashrc
        cp /tmp/dotfiles_src/.test_vimrc /home/testuser/dotfiles_src/.test_vimrc
        # Create valid symlinks
        ln -sf /home/testuser/dotfiles_src/.test_bashrc /home/testuser/.actual_bashrc
        ln -sf /home/testuser/dotfiles_src/.test_vimrc /home/testuser/.actual_vimrc
        # Create broken symlink (source does not exist)
        ln -sf /home/testuser/dotfiles_src/.nonexistent_source /home/testuser/.broken_target
        # .config exists, .missing_test_dir does not
    "

# Run dotter doctor and capture output (expect non-zero exit)
echo ""
echo "Running dotter doctor..."
set +e
DOCTOR_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} doctor 2>&1)
DOCTOR_EXIT=$?
set -e

echo "Doctor output:"
echo "${DOCTOR_OUTPUT}"
echo ""
echo "Doctor exit code: ${DOCTOR_EXIT}"

# Verify summary section exists
if ! echo "${DOCTOR_OUTPUT}" | grep -qF -- '--- Summary ---'; then
    echo "ERROR: Output does not contain '--- Summary ---'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Summary section present"

# Verify FAIL appears for broken symlink
if ! echo "${DOCTOR_OUTPUT}" | grep -q 'FAIL.*broken_link'; then
    echo "ERROR: Output does not contain FAIL for broken_link"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: FAIL for broken_link present"

# Verify WARN appears for missing tool
if ! echo "${DOCTOR_OUTPUT}" | grep -q 'WARN.*nonexistent_tool_xyz'; then
    echo "ERROR: Output does not contain WARN for nonexistent_tool_xyz"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: WARN for nonexistent_tool_xyz present"

# Verify exit code is 1 (has failures)
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
echo "  - Summary section present"
echo "  - FAIL shown for broken symlink"
echo "  - WARN shown for missing tool"
echo "  - Exit code 1 for failures"
