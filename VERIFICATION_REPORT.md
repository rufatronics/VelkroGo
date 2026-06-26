# VelkroGo v1.1.0 Verification Report

## Overview
Verified Phase 1 claims of VelkroGo. The core architecture and most Phase 1 features are implemented and functional. Critical issues in the daemon, setup wizard, and GUI have been fixed.

## Findings

### 1. Build & Installation
- ✅ `velkrod` (daemon) and `velkro` (TUI) build successfully on Linux.
- ✅ Dependency management via Go modules is healthy.
- ℹ️ `velkroapp` (GUI) requires X11/OpenGL headers as documented.

### 2. Phase 1 Claims Verification
- ✅ **Provider Adapters:** Anthropic, Gemini, and OpenAI-compatible adapters are present and functional.
- ✅ **Agent Loop:** ReAct-style loop is implemented in `internal/orchestrator`.
- ✅ **Visible Plan:** `set_plan` tool allows the agent to display a live plan in the TUI/GUI.
- ✅ **Safety Core:** Policy engine correctly gates tools by risk tier (T0-T4).
- ✅ **Question Box:** `ask_user` mechanism allows for human-in-the-loop reasoning.
- ✅ **TUI:** Bubble Tea TUI correctly renders chat, plans, and approval/question modals.
- ✅ **GUI:** Fyne-based GUI verified for feature parity (chat, plan, approvals, questions).
- ✅ **Cost Modes:** Saver/Normal modes correctly switch system prompts.
- ✅ **Grants:** "Allow for session" correctly persists grants in memory for the duration of the session.

### 3. Identified Issues (Now Fixed)

#### 🔴 Fixed: Daemon Request Cancellation
The `/api/run` endpoint in `internal/api/server.go` was using `r.Context()`. This caused tasks to be cancelled immediately after the HTTP request completed. Switched to `context.Background()`.

#### 🟡 Fixed: Setup Wizard Input Handling & Defaults
The first-run wizard in `cmd/velkro/main.go` and `cmd/velkroapp/main.go` was fragile and had opinionated defaults. Robustified input trimming and removed default models to force user selection.

#### 🟡 Fixed: GUI Scrollability
Long tool previews in approval dialogs and many options in question dialogs would overflow the window. Wrapped these in scrollable containers.

#### ℹ️ Note on v1.1.0 vs v1.0.1
`v1.1.0` is a minor update over `v1.0.1`, primarily adding an ignore rule for `.claude/` directories. All core functionality remains consistent across these versions.

## Recommendations
1. Add a health check endpoint `/api/health` to the daemon for easier monitoring.
