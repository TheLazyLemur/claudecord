#!/bin/bash
set -e

# Usage: send.sh <to> <subject> <body> [from] [attachments]
TO="$1"
SUBJECT="$2"
BODY="$3"
FROM="${4:-assistant@assist.opsbox.co.za}"
ATTACHMENTS="$5"

if [ -z "$RESEND_API_KEY" ]; then
  echo "Error: RESEND_API_KEY environment variable not set" >&2
  exit 1
fi

if [ -z "$TO" ] || [ -z "$SUBJECT" ] || [ -z "$BODY" ]; then
  echo "Usage: send.sh <to> <subject> <body> [from] [attachments]" >&2
  echo "  to:          Recipient email address" >&2
  echo "  subject:     Email subject line" >&2
  echo "  body:        Email body text" >&2
  echo "  from:        Sender address (default: assistant@assist.opsbox.co.za)" >&2
  echo "  attachments: Comma-separated file paths (optional)" >&2
  exit 1
fi

# Build base JSON payload
JSON=$(jq -n \
  --arg from "$FROM" \
  --arg to "$TO" \
  --arg subject "$SUBJECT" \
  --arg text "$BODY" \
  '{from: $from, to: $to, subject: $subject, text: $text}')

# Add attachments if provided
if [ -n "$ATTACHMENTS" ]; then
  ATTACH_JSON="[]"
  IFS=',' read -ra FILES <<< "$ATTACHMENTS"
  for file in "${FILES[@]}"; do
    file=$(echo "$file" | xargs)  # trim whitespace
    if [ ! -f "$file" ]; then
      echo "Error: Attachment not found: $file" >&2
      exit 1
    fi
    filename=$(basename "$file")
    content=$(base64 < "$file")
    ATTACH_JSON=$(echo "$ATTACH_JSON" | jq \
      --arg fn "$filename" \
      --arg ct "$content" \
      '. + [{filename: $fn, content: $ct}]')
  done
  JSON=$(echo "$JSON" | jq --argjson att "$ATTACH_JSON" '. + {attachments: $att}')
fi

# Send via Resend API
RESPONSE=$(curl -s -X POST 'https://api.resend.com/emails' \
  -H "Authorization: Bearer $RESEND_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "$JSON")

# Check for error
if echo "$RESPONSE" | jq -e '.statusCode' > /dev/null 2>&1; then
  ERROR=$(echo "$RESPONSE" | jq -r '.message')
  echo "Error: $ERROR" >&2
  exit 1
fi

# Success - extract ID
EMAIL_ID=$(echo "$RESPONSE" | jq -r '.id')
echo "Email sent successfully"
echo "  To: $TO"
echo "  Subject: $SUBJECT"
echo "  ID: $EMAIL_ID"
