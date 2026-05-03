#!/bin/bash
set -e

if [ -z "$MEMORY_DIR" ]; then
  echo "Error: MEMORY_DIR environment variable not set" >&2
  exit 1
fi

PATTERN="$1"
if [ -z "$PATTERN" ]; then
  echo "Usage: search.sh <pattern>" >&2
  exit 1
fi

if [ ! -d "$MEMORY_DIR" ]; then
  echo "(no memory dir yet)"
  exit 0
fi

# -r recurse, -n line numbers, -i case-insensitive, -I skip binary
# Suppress exit code 1 (no matches) so the model gets a clean message.
MATCHES=$(grep -rniI -- "$PATTERN" "$MEMORY_DIR" 2>/dev/null || true)

if [ -z "$MATCHES" ]; then
  echo "(no matches for: $PATTERN)"
  exit 0
fi

# Strip the absolute prefix so output is shorter.
echo "$MATCHES" | sed "s|^$MEMORY_DIR/||"
