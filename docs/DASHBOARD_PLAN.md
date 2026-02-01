# Claudecord Web Dashboard - Implementation Plan

## Overview
A web-based dashboard for managing the Claudecord Discord bot, providing real-time monitoring, configuration management, and web-based chat interface.

## Features

### 1. Real-time Logs View
- WebSocket connection for streaming logs
- Log levels filter (DEBUG, INFO, WARN, ERROR)
- Search/filter capabilities
- Export logs to file

### 2. Permissions Settings
- View current allowed directories and users
- Add/remove directories with validation
- Add/remove Discord users by ID
- Changes persist to config

### 3. Create New Sessions
- List active sessions
- Create new session with optional working directory
- Terminate specific sessions

### 4. Continue Chats in Web Browser
- Chat interface via web
- Real-time message streaming via WebSocket
- Support for tool approvals
- Session history view

## Architecture

### New Components
- internal/dashboard/server.go
- internal/dashboard/websocket.go
- internal/dashboard/auth.go
- internal/dashboard/handlers.go
- internal/dashboard/static/ (embedded web assets)

### API Endpoints
- GET /api/health - Health check
- GET /api/logs - Log stream (WebSocket)
- GET /api/config - Get config
- PUT /api/config - Update config
- GET /api/sessions - List sessions
- POST /api/sessions - Create session
- DELETE /api/sessions/:id - Terminate session
- POST /api/chat - Send message (WebSocket)

## Implementation Phases
1. Foundation (MVP) - Server, auth, basic UI, logs
2. Configuration Management - Config API, permissions UI
3. Session Management - Session API, list UI
4. Web Chat - Chat protocol, UI, tool approvals

## Files to Create
- internal/dashboard/*.go
- internal/dashboard/static/*
- internal/config/persistence.go

## Files to Modify
- cmd/claudecord/main.go
- internal/config/config.go
- internal/core/bot.go
- go.mod

## New Dependencies
github.com/gorilla/websocket v1.5.1

## New Config
DASHBOARD_ENABLED=true
DASHBOARD_PORT=8080
