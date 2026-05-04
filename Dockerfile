# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /claudecord ./cmd/claudecord

# Runtime stage
FROM node:24-slim

# Install system dependencies:
# - bash, zsh, git, openssh-client, curl, jq (general tooling)
# - poppler-utils (pdftotext for pdf-reader skill)
# - pandoc (docx-reader and link-summarize skills)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       bash zsh git openssh-client curl jq \
       poppler-utils pandoc \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI (gh) — not in default Debian repos, use official apt repo
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy Go binary
COPY --from=builder /claudecord /usr/local/bin/claudecord

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Custom entrypoint to symlink config dirs and auth gh
RUN cat <<'ENTRYPOINT' > /entrypoint.sh
#!/bin/sh
mkdir -p /root/.claude/.config
ln -sf /root/.claude/.config /root/.config
ln -sf /root/.claude/.claude.json /root/.claude.json

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
