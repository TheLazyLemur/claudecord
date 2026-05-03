#!/bin/bash
set -e

# Usage: extract.sh <path> [first-page] [last-page]
PATH_ARG="$1"
FIRST_PAGE="$2"
LAST_PAGE="$3"

if [ -z "$PATH_ARG" ]; then
  echo "Usage: extract.sh <path> [first-page] [last-page]" >&2
  exit 1
fi

if ! command -v pdftotext >/dev/null 2>&1; then
  echo "Error: pdftotext not installed (need poppler-utils)" >&2
  exit 1
fi

if [ ! -f "$PATH_ARG" ]; then
  echo "Error: file not found: $PATH_ARG" >&2
  exit 1
fi

ARGS=(-layout)
if [ -n "$FIRST_PAGE" ]; then
  ARGS+=(-f "$FIRST_PAGE")
fi
if [ -n "$LAST_PAGE" ]; then
  ARGS+=(-l "$LAST_PAGE")
fi

# `-` writes to stdout
pdftotext "${ARGS[@]}" "$PATH_ARG" -
