# VelkroGo Agent Knowledge Base (AGENTS.md)

This file serves as the primary context and memory for AI agents working on the VelkroGo repository. It outlines the project's structure, architecture, and current state.

## 🚀 Project Overview
VelkroGo is a self-hostable, cross-platform AI agent written in Go. It fuses a coding agent and a general-purpose computer-use agent behind a safety-first core.

### Key Components:
- **`cmd/velkrod`**: The background daemon. It hosts the agent loop, scheduler, and API.
- **`cmd/velkro`**: The TUI (Bubble Tea) client.
- **`cmd/velkroapp`**: The GUI (Fyne) client.
- **`internal/orchestrator`**: The core ReAct loop and planning logic.
- **`internal/policy`**: The risk-tiered (T0-T4) approval engine.
- **`internal/api`**: The local HTTP+WS API (default `127.0.0.1:7477`).

## 🛠 Repository Structure
- `cmd/`: Entry points for daemon, TUI, and GUI.
- `internal/`: Core logic packages.
    - `api/`: REST and WebSocket server.
    - `audit/`: Append-only local logs.
    - `config/`: Config loading and persistence (`~/.config/velkrogo/`).
    - `memory/`: SQLite state management.
    - `orchestrator/`: The agent's "brain" and turn-based loop.
    - `policy/`: Safety gate for tool execution.
    - `provider/`: Adapters for LLMs (Anthropic, Gemini, OpenAI-compat).
    - `registry/`: Central tool/capability registry.
    - `tools/`: Built-in T0/T1 tools (FS, Web, etc.).
    - `worlds/`: Specialized toolsets (Coder/Operator).

## 🛡 Security & Safety
- **Risk Tiers**:
    - **T0**: Read-only (Auto-allow).
    - **T1**: Local reversible (Write file, git commit).
    - **T2**: External (Git push, HTTP POST).
    - **T3**: Device control (Shell, Mouse/Keyboard).
    - **T4**: Self-modification.
- **Policy Gate**: All tools MUST pass through `internal/policy` before execution.
- **Context Management**: Daemon tasks use `context.Background()` to persist after HTTP requests finish.

## 🤖 Instructions for Agents
1. **Tool Registration**: When adding new tools, register them in `cmd/velkro/main.go`, `cmd/velkrod/main.go`, and `cmd/velkroapp/main.go`.
2. **Context**: Always use the provided context for tool execution to support cancellation.
3. **No Defaults**: Do not hardcode default models in the setup wizard; force users to specify their preferred model.
4. **API Stability**: Maintain compatibility with the existing WebSocket event structure for frontends.

## ✅ Current Status (Phase 1 Complete)
- Stable ReAct loop with visible planning.
- Functional safety core and tiered approvals.
- TUI and Daemon are primary verified interfaces.
- GUI (Fyne) is implemented but requires native headers for building.

## 📝 Recent Fixes (June 2026)
- Fixed daemon task cancellation bug by switching to `context.Background()` for async runs.
- Robustified TUI setup wizard input handling.
- Removed opinionated default models from all setup wizards.
- Added lazy provider loading to the API server.
