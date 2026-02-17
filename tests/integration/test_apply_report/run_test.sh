#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="dotter-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Apply produces correct summary with mixed outcomes ==="

VOLUME_NAME="dotter-test-apply-report-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Run dotter apply and capture output (expect non-zero exit due to broken dotfile)
echo ""
echo "Running dotter apply..."
set +e
APPLY_OUTPUT=$(docker run --rm \
    -v "${TEST_CASE_DIR}/config.toml:/home/testuser/.config/dotter/config.toml:ro" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/home/testuser/dotfiles_src:ro" \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} apply 2>&1)
APPLY_EXIT=$?
set -e

echo "Apply output:"
echo "${APPLY_OUTPUT}"
echo ""
echo "Apply exit code: ${APPLY_EXIT}"

# Verify summary section exists
if ! echo "${APPLY_OUTPUT}" | grep -qF -- '--- Summary ---'; then
    echo "ERROR: Output does not contain '--- Summary ---'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Summary section present"

# Verify Dotfiles phase appears with counts
if ! echo "${APPLY_OUTPUT}" | grep -q 'Dotfiles:'; then
    echo "ERROR: Output does not contain Dotfiles phase"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Dotfiles phase present"

# Verify FAIL appears for broken dotfile
if ! echo "${APPLY_OUTPUT}" | grep -q 'FAIL.*broken_dotfile'; then
    echo "ERROR: Output does not contain FAIL for broken_dotfile"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: FAIL for broken_dotfile present"

# Verify "ok" count appears in totals
if ! echo "${APPLY_OUTPUT}" | grep -q 'ok'; then
    echo "ERROR: Output does not contain ok count in totals"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: OK count in totals"

# Verify exit code reflects failures (should be 1)
if [ "$APPLY_EXIT" -ne 1 ]; then
    echo "ERROR: Expected exit code 1 (has failures), got ${APPLY_EXIT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
echo "CHECK: Exit code is 1 (has failures)"

# Clean up
echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: Apply report output verified ==="
echo "  - Summary section present"
echo "  - Dotfiles phase with counts present"
echo "  - FAIL shown for broken dotfile"
echo "  - OK count in totals"
echo "  - Exit code 1 for failures"
