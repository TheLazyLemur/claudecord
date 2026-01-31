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

# Custom entrypoint to symlink config dirs
RUN printf '#!/bin/sh\nmkdir -p /root/.claude/.config\nln -sf /root/.claude/.config /root/.config\nln -sf /root/.claude/.claude.json /root/.claude.json\nexec claudecord\n' > /entrypoint.sh && chmod +x /entrypoint.sh

# Default port for webhook server
ENV WEBHOOK_PORT=8080
ENV SHELL=/bin/bash

EXPOSE 8080

CMD ["/entrypoint.sh"]
