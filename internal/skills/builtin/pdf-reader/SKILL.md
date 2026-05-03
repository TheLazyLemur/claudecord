---
name: pdf-reader
description: Extract text from a PDF attachment. Use this skill when the user message contains an <attachment> tag whose mime attribute is application/pdf.
---

# PDF Reader

Extracts plain text from a PDF file using `pdftotext` (poppler-utils).

## When to Use

You see an `<attachment>` tag in the user message with `mime="application/pdf"`. Pick up the `path` attribute and pass it to the script.

## Usage

```bash
bash scripts/extract.sh <path>
```

Optionally limit pages (useful for very large PDFs):

```bash
bash scripts/extract.sh <path> <first-page> <last-page>
```

The script prints the extracted text to stdout. If extraction fails (corrupt PDF, encrypted, unsupported), it exits non-zero with the error on stderr.

## Steps

1. Read the `path` attribute from the `<attachment mime="application/pdf" ... />` tag.
2. Run `bash scripts/extract.sh <path>` via the Bash tool.
3. Use the extracted text to answer the user. If the PDF is long, summarize; if the user asked a specific question, quote the relevant passages.
4. If extraction fails, tell the user the PDF couldn't be parsed and pass on the error.

## Requirements

- `pdftotext` binary (provided by `poppler-utils` in the runtime image).
