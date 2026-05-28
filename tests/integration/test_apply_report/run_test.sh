#!/bin/bash
set -e

# Get the absolute path to the project root
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: Apply produces correct summary with mixed outcomes ==="

VOLUME_NAME="ralph-test-apply-report-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Run ralph up --no-sync and capture JSON stdout only (expect non-zero exit due to broken dotfile)
echo ""
echo "Running ralph up --no-sync -o json..."
set +e
JSON=$(docker run --rm \
    -v "${TEST_CASE_DIR}/config.toml:/home/testuser/.config/ralph/config.toml:ro" \
    -v "${TEST_CASE_DIR}/dotfiles_src:/home/testuser/dotfiles_src:ro" \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync -o json 2>/dev/null)
APPLY_EXIT=$?
set -e

echo "JSON output:"
echo "${JSON}"
echo ""
echo "Apply exit code: ${APPLY_EXIT}"

# Assert at least one failure in summary
echo "$JSON" | jq -e '.summary.failed >= 1' >/dev/null || {
    echo "ERROR: Expected summary.failed >= 1"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: summary.failed >= 1"

# Assert a step named "broken_dotfile" with status "fail"
echo "$JSON" | jq -e '[.phases[].steps[]|select(.name=="broken_dotfile" and .status=="fail")]|length>=1' >/dev/null || {
    echo "ERROR: No step named 'broken_dotfile' with status 'fail'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: step broken_dotfile with status fail present"

# Assert exit_code field in JSON is 1
echo "$JSON" | jq -e '.exit_code == 1' >/dev/null || {
    echo "ERROR: JSON exit_code is not 1"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: JSON exit_code == 1"

# Verify captured exit code reflects failures (should be 1)
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
echo "  - summary.failed >= 1"
echo "  - step broken_dotfile with status fail"
echo "  - JSON exit_code == 1"
echo "  - process exit code 1 for failures"
