#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

mkdir -p "$MEMORY_DIR/daily"

TODAY=$(date -u +%Y-%m-%d)
YESTERDAY=$(date -u -d "yesterday" +%Y-%m-%d 2>/dev/null || date -u -v-1d +%Y-%m-%d)

print_section() {
  local label="$1"
  local file="$2"
  if [ -s "$file" ]; then
    echo "=== $label ($file) ==="
    cat "$file"
    echo
  else
    echo "=== $label ($file): empty ==="
    echo
  fi
}

print_section "MEMORY.md" "$MEMORY_DIR/MEMORY.md"
print_section "today" "$MEMORY_DIR/daily/$TODAY.md"
print_section "yesterday" "$MEMORY_DIR/daily/$YESTERDAY.md"
