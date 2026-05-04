#!/bin/sh
# Test fixture: increments /home/testuser/.idem_counter by 1 each call.
# The integration test asserts the counter only changes when the build
# actually runs (i.e., is not skipped by the idempotent short-circuit).
COUNTER_FILE="/home/testuser/.idem_counter"
COUNT=$(cat "$COUNTER_FILE" 2>/dev/null || echo 0)
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"
echo "bump_counter -> $COUNT"
