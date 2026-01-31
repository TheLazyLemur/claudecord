---
name: receive-email
description: Check and read received emails via Resend API. Use when the user asks to check inbox, read emails, or see what emails were received.
---

# Receive Email

Check and read inbound emails via Resend API on `assist.opsbox.co.za`.

## List Recent Emails

```bash
bash scripts/list.sh [limit]
```

Returns list of received emails with sender, subject, date, and ID.

## Read Specific Email

```bash
bash scripts/read.sh <email_id>
```

Returns full email content including body.

## Workflow

1. First run `list.sh` to see recent emails
2. Use the ID from the list to read a specific email with `read.sh`

## Requirements

- `RESEND_API_KEY` environment variable must be set
- `jq` and `curl` must be installed
