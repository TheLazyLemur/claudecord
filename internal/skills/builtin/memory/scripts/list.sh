#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

if [ ! -d "$MEMORY_DIR" ]; then
  echo "(no memory dir yet)"
  exit 0
fi

cd "$MEMORY_DIR"

FILES=$(find . -type f | sed 's|^\./||' | sort)

if [ -z "$FILES" ]; then
  echo "(memory dir is empty)"
  exit 0
fi

while IFS= read -r rel; do
  size=$(wc -c < "$rel" | tr -d ' ')
  printf '%s\t%s bytes\n' "$rel" "$size"
done <<< "$FILES"
