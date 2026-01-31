#!/bin/bash
set -e

# Usage: list.sh [limit]
LIMIT="${1:-10}"

if [ -z "$RESEND_API_KEY" ]; then
  echo "Error: RESEND_API_KEY environment variable not set" >&2
  exit 1
fi

RESPONSE=$(curl -s -X GET "https://api.resend.com/emails/receiving?limit=$LIMIT" \
  -H "Authorization: Bearer $RESEND_API_KEY")

# Check for error
if echo "$RESPONSE" | jq -e '.statusCode' > /dev/null 2>&1; then
  ERROR=$(echo "$RESPONSE" | jq -r '.message')
  echo "Error: $ERROR" >&2
  exit 1
fi

# Format output
echo "$RESPONSE" | jq -r '.data[] | "[\(.created_at)] From: \(.from) | To: \(.to) | Subject: \(.subject) | ID: \(.id)"'
