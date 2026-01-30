# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /claudecord ./cmd/claudecord

# Runtime stage
FROM node:20-alpine

# Install bash, zsh, git, and gh CLI
RUN apk add --no-cache bash zsh git github-cli

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy Go binary
COPY --from=builder /claudecord /usr/local/bin/claudecord

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Default port for webhook server
ENV WEBHOOK_PORT=8080
ENV SHELL=/bin/bash

EXPOSE 8080

CMD ["claudecord"]
