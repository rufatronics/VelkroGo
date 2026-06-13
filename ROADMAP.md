# VelkroGo — Build Roadmap (phased steps)

Each phase is shippable and demoable on its own. Safety primitives come *before*
power, on purpose — we never add a dangerous capability before the gate that
controls it exists.

## Phase 0 — Skeleton & contracts *(foundation)*
- Go module, repo layout (see ARCHITECTURE §9), CI cross-compiling Win+Linux.
- Core interfaces only (no logic): `Provider`, `Tool`/`Capability`, `Policy`,
  `Executor`, event types, the daemon's local API surface.
- SQLite state bootstrap + config/secrets loading.
- **Demo:** `velkrod` starts, `velkro` connects, empty session over WS.

## Phase 1 — Brain + loop + TUI
- Anthropic + OpenAI-compatible providers; model routing.
- Orchestrator ReAct loop with a **visible plan/step list**.
- TUI (Bubble Tea) chat + plan view.
- A couple of T0 tools (echo, read file) to prove tool routing.
- **Demo:** chat, watch a live plan, call read-only tools.

## Phase 2 — Safety core
- Policy & approval engine with scoped, revocable grants + previews.
- The **Question Box** (`ask_user`) reasoning mechanism.
- Append-only hash-chained audit log + viewer.
- Pause / kill-switch / revoke-all.
- **Demo:** any side-effecting tool prompts for scoped approval; agent asks
  clarifying questions before ambiguous steps.

## Phase 3 — World 1 (Coder)
- Sandbox executor (container backend + host fallback).
- git + GitHub/GitLab tools; project workspace; build/test/run runners.
- **Demo:** "fix this bug & open a PR" end to end with approvals.

## Phase 4 — Search + Browser
- DuckDuckGo scrape + readability fetch (T0).
- CDP browser control with **human takeover / resume**.
- **Demo:** cheap web research; then a browser task with a manual login handoff.

## Phase 5 — World 2 (Operator)
- Shell/fs beyond workspace, GUI control, OS adapters (Win/Linux), device
  connectors — all behind the gate.
- **Demo:** an everyday desktop task with per-action approval.

## Phase 6 — Scheduler + Cost modes
- One-shot + recurring jobs, persisted, run headless; NL time parsing.
- Saver vs. Normal mode switch (single-agent/minimal prompt vs. sub-agents).
- **Demo:** "every 15 min, do X"; flip to saver mode and watch token/tool use drop.

## Phase 7 — Lightweight GUI
- Local web UI: chat, plan, question box, approval previews, browser pane +
  takeover, audit, schedules, settings.
- **Demo:** full feature parity with TUI in the GUI.

## Phase 8 — Skills + Self-modification
- Skill authoring/install (additive).
- Self-edit core: diff review → accept-risk toggle → rebuild → self-test →
  snapshot → hot-restart → auto-rollback on failure.
- **Demo:** agent adds a skill; then proposes & safely applies a self-improvement.

## Phase 9 — Hardening & release
- Secrets in OS keychain, egress allowlist, fuzzing the policy gate, docs,
  signed release binaries for Windows + Linux.
