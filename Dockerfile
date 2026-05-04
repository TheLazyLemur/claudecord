# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /claudecord ./cmd/claudecord

# Runtime stage
FROM debian:bookworm-slim

# Install system dependencies:
# - ca-certificates (TLS trust store for the Go binary, built CGO_ENABLED=0)
# - bash, zsh, git, openssh-client, curl, jq (general tooling)
# - poppler-utils (pdftotext for pdf-reader skill)
# - pandoc (docx-reader and link-summarize skills)
# - chromium runtime libs + fonts (Playwright headless Chromium for PDF/render skills)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       bash zsh git openssh-client curl jq \
       poppler-utils pandoc \
       libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
       libdrm2 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 \
       libgbm1 libxkbcommon0 libpango-1.0-0 libcairo2 libasound2 \
       fonts-liberation fonts-noto-color-emoji \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI (gh) — not in default Debian repos, use official apt repo
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Copy Go binary
COPY --from=builder /claudecord /usr/local/bin/claudecord

WORKDIR /root

# Custom entrypoint to auth gh and configure git
RUN cat <<'ENTRYPOINT' > /entrypoint.sh
#!/bin/sh
# Ensure workspace dir exists on the mounted volume
mkdir -p /root/workspace

# Auth gh CLI if GH_TOKEN is set
if [ -n "$GH_TOKEN" ]; then
  echo "$GH_TOKEN" | gh auth login --with-token
  gh auth setup-git
fi

# Git config if set
if [ -n "$GIT_USER_NAME" ]; then
  git config --global user.name "$GIT_USER_NAME"
fi
if [ -n "$GIT_USER_EMAIL" ]; then
  git config --global user.email "$GIT_USER_EMAIL"
fi

exec claudecord
ENTRYPOINT
RUN chmod +x /entrypoint.sh

# Default port for webhook server
ENV WEBHOOK_PORT=8080
ENV SHELL=/bin/bash

EXPOSE 8080

CMD ["/entrypoint.sh"]
