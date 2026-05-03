#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

TEXT="$1"
if [ -z "$TEXT" ]; then
  echo "Usage: note.sh <text>" >&2
  exit 1
fi

mkdir -p "$MEMORY_DIR/daily"
TODAY=$(date -u +%Y-%m-%d)
TIME=$(date -u +%H:%M)
FILE="$MEMORY_DIR/daily/$TODAY.md"

if [ ! -s "$FILE" ]; then
  printf '# %s\n\n' "$TODAY" > "$FILE"
fi

LINE="- ${TIME}Z $TEXT"
printf '%s\n' "$LINE" >> "$FILE"
echo "Wrote to $FILE: $LINE"
