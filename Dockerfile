# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /claudecord ./cmd/claudecord

# Runtime stage
FROM node:20-alpine

# Install bash, zsh, git, gh CLI, and openssh
RUN apk add --no-cache bash zsh git github-cli openssh-client curl jq

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy Go binary
COPY --from=builder /claudecord /usr/local/bin/claudecord

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Custom entrypoint to symlink config dirs and auth gh
RUN cat <<'EOF' > /entrypoint.sh
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
EOF
RUN chmod +x /entrypoint.sh

# Default port for webhook server
ENV WEBHOOK_PORT=8080
ENV SHELL=/bin/bash

EXPOSE 8080

CMD ["/entrypoint.sh"]
