#!/bin/bash
# Integration test: --dry-run does not mutate the filesystem.
#
# Runs ralph up --no-sync --dry-run -o json and asserts:
#   1. JSON field dry_run == true
#   2. JSON field exit_code == 0
#   3. The target symlink/file does NOT exist on disk afterward.
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="ralph-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/Dockerfile

echo "=== TEST: --dry-run makes no filesystem changes ==="

VOLUME_NAME="ralph-test-dryrun-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Seed a minimal config with one dotfile
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/.config/ralph /home/testuser/dotfiles_src
        printf '%s\n' \
            'dotfiles_repo_path = \"/home/testuser/dotfiles_src\"' \
            '' \
            '[dotfiles.dryrun_test]' \
            '  source = \".dryrun_test\"' \
            '  target = \"/home/testuser/.dryrun_target\"' \
            > /home/testuser/.config/ralph/config.toml
        echo 'dry run content' > /home/testuser/dotfiles_src/.dryrun_test
    "

# Run ralph up --no-sync --dry-run -o json; capture stdout only
echo ""
echo "Running ralph up --no-sync --dry-run -o json..."
JSON=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} up --no-sync --dry-run -o json 2>/dev/null)
echo "${JSON}"

# Assert dry_run == true
echo "$JSON" | jq -e '.dry_run == true' >/dev/null || {
    echo "ERROR: JSON field dry_run is not true"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: dry_run == true"

# Assert exit_code == 0
echo "$JSON" | jq -e '.exit_code == 0' >/dev/null || {
    echo "ERROR: JSON field exit_code is not 0"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
}
echo "CHECK: exit_code == 0"

# Assert target does NOT exist (dry-run must not create it)
echo ""
echo "Verifying target symlink was NOT created..."
docker run --rm \
    --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        if [ -e /home/testuser/.dryrun_target ] || [ -L /home/testuser/.dryrun_target ]; then
            echo 'ERROR: .dryrun_target exists after --dry-run — filesystem was mutated!'
            exit 1
        fi
        echo 'PASS: .dryrun_target does not exist (dry-run made no changes)'
    "

echo ""
echo "Cleaning up volume ${VOLUME_NAME}..."
docker volume rm ${VOLUME_NAME} > /dev/null

echo ""
echo "=== TEST PASSED: --dry-run makes no filesystem changes ==="
echo "  - dry_run == true in JSON"
echo "  - exit_code == 0"
echo "  - target symlink absent from volume"
