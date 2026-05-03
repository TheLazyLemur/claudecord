#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

TEXT="$1"
if [ -z "$TEXT" ]; then
  echo "Usage: remember.sh <text>" >&2
  exit 1
fi

mkdir -p "$MEMORY_DIR"
FILE="$MEMORY_DIR/MEMORY.md"
touch "$FILE"

DATE=$(date -u +%Y-%m-%d)
LINE="- ($DATE) $TEXT"

if grep -Fxq "$LINE" "$FILE" 2>/dev/null; then
  echo "Already in MEMORY.md (skipped duplicate)"
  exit 0
fi

# Also dedupe if the same TEXT exists with a different date prefix.
ESCAPED=$(printf '%s\n' "$TEXT" | sed 's/[][\.*^$/]/\\&/g')
if grep -Eq "^- \([0-9]{4}-[0-9]{2}-[0-9]{2}\) ${ESCAPED}\$" "$FILE" 2>/dev/null; then
  echo "Already in MEMORY.md under a previous date (skipped)"
  exit 0
fi

printf '%s\n' "$LINE" >> "$FILE"
echo "Wrote to $FILE: $LINE"
