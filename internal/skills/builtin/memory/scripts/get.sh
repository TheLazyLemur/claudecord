#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

REL="$1"
START="$2"
END="$3"

if [ -z "$REL" ]; then
  echo "Usage: get.sh <relative-path> [start-line] [end-line]" >&2
  exit 1
fi

case "$REL" in
  /*|*..*)
    echo "Error: path must be relative and cannot contain .." >&2
    exit 1
    ;;
esac

FILE="$MEMORY_DIR/$REL"
if [ ! -f "$FILE" ]; then
  echo "Error: file not found: $REL" >&2
  exit 1
fi

if [ -z "$START" ]; then
  cat "$FILE"
  exit 0
fi

if [ -z "$END" ]; then
  END="$START"
fi

awk -v s="$START" -v e="$END" 'NR>=s && NR<=e' "$FILE"
