#!/bin/sh
# This script creates/increments a counter file to track how many times it runs
COUNTER_FILE="/home/testuser/.build_counter"

if [ -f "$COUNTER_FILE" ]; then
    COUNT=$(cat "$COUNTER_FILE")
    COUNT=$((COUNT + 1))
else
    COUNT=1
fi

echo "$COUNT" > "$COUNTER_FILE"
echo "Build script executed. Run count: $COUNT"
