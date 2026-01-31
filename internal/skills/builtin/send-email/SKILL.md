---
name: send-email
description: Send an email via Resend API. Use when the user asks to send an email, notify someone, or communicate via email.
---

# Send Email

Send emails via Resend API from `assist.opsbox.co.za`.

## Usage

```bash
bash scripts/send.sh <to> <subject> <body> [from] [attachments]
```

**Arguments:**
- `to` - Recipient email address (required)
- `subject` - Email subject line (required)
- `body` - Email body text (required)
- `from` - Sender address (optional, defaults to `assistant@assist.opsbox.co.za`)
- `attachments` - Comma-separated file paths (optional)

## Examples

```bash
# Simple email
bash scripts/send.sh "dan@example.com" "Hello" "This is a test"

# Custom sender
bash scripts/send.sh "dan@example.com" "Alert" "Server is down" "alerts@assist.opsbox.co.za"

# With attachments
bash scripts/send.sh "dan@example.com" "Report" "See attached" "assistant@assist.opsbox.co.za" "/path/to/report.pdf"

# Multiple attachments
bash scripts/send.sh "dan@example.com" "Files" "Here are the files" "assistant@assist.opsbox.co.za" "/path/file1.pdf,/path/file2.txt"
```

## Requirements

- `RESEND_API_KEY` environment variable must be set
- `jq` and `curl` must be installed

## When Invoked

Parse user's request to extract recipient, subject, body, and any attachments. If required fields are missing, ask the user. For attachments, use absolute paths or resolve relative paths from user's working directory.
