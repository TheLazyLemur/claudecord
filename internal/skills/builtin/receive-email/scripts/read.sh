#!/bin/bash
set -e

# Usage: read.sh <email_id>
EMAIL_ID="$1"

if [ -z "$RESEND_API_KEY" ]; then
  echo "Error: RESEND_API_KEY environment variable not set" >&2
  exit 1
fi

if [ -z "$EMAIL_ID" ]; then
  echo "Usage: read.sh <email_id>" >&2
  echo "  Get email ID from: list.sh" >&2
  exit 1
fi

RESPONSE=$(curl -s -X GET "https://api.resend.com/emails/receiving/$EMAIL_ID" \
  -H "Authorization: Bearer $RESEND_API_KEY")

# Check for error
if echo "$RESPONSE" | jq -e '.statusCode' > /dev/null 2>&1; then
  ERROR=$(echo "$RESPONSE" | jq -r '.message')
  echo "Error: $ERROR" >&2
  exit 1
fi

# Format output
echo "=== Email Details ==="
echo "$RESPONSE" | jq -r '"From: \(.from)\nTo: \(.to)\nSubject: \(.subject)\nDate: \(.created_at)\nMessage-ID: \(.message_id)"'
echo ""
echo "=== Body ==="
# Try to get text, fallback to html
TEXT=$(echo "$RESPONSE" | jq -r '.text // empty')
if [ -n "$TEXT" ]; then
  echo "$TEXT"
else
  HTML=$(echo "$RESPONSE" | jq -r '.html // empty')
  if [ -n "$HTML" ]; then
    echo "$HTML" | sed 's/<[^>]*>//g' | head -50
  else
    echo "(no body)"
  fi
fi
