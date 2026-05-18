# playerws — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/playerws/hub.go` (269 LOC) — 🆕 NEW in v2.8.0
> **Last updated:** 2026-05-17

## Purpose

WebSocket hub for multi-device playback control. Allows remote devices (phones, other browsers) to control playback and receive state updates.

## Architecture

- `Hub` struct manages connected clients and broadcasts state changes
- `Client` struct represents a single WebSocket connection
- Uses `gorilla/websocket` for WebSocket protocol

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/ws/player` | WebSocket endpoint for player control |

## Features

- Device discovery — clients announce themselves on connect
- Playback control — play, pause, next, prev, seek, volume
- State sync — current track, position, play state broadcast to all clients
- Device transfer — transfer playback to another device

## Working Here

- Adding new control commands: edit `hub.go` message handlers
- Adding device types: extend the client registration protocol
