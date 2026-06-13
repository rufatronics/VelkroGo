# VelkroGo

A self-hostable, cross-platform (Windows + Linux) AI agent in Go that fuses two
"worlds" behind one safety-first core:

- **World 1 — Coder:** connects to GitHub/GitLab and other dev services, manages
  code across repos, builds/tests, and ships full apps.
- **World 2 — Operator:** drives the actual device — files, shell, apps,
  browser, network — for everyday tasks.

Every powerful action is gated behind **explicit, scoped, revocable user
approval**. VelkroGo can **schedule** work, **search the web cheaply** or drive
a **real browser with human takeover**, reason **Claude-style** (a question box
appears before it proceeds when uncertain), run a **money-saving mode**, support
**any AI provider** (including custom ones), and even **add skills / improve its
own code** — only when you accept the risk.

> **Status: design phase.** This repo currently contains the architecture, the
> phased roadmap, and a Phase 0 Go skeleton (core interfaces + daemon/client
> entry points). No agent behaviour is wired yet.

## Read first
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — full design, components, security
  model, repo layout, and open decisions.
- [`ROADMAP.md`](./ROADMAP.md) — phased, demoable build plan.

## Key ideas at a glance
- **Risk tiers (T0–T4)** drive every approval decision.
- **Daemon + thin frontends:** one `velkrod` daemon, with a lightweight web GUI
  and a Bubble Tea TUI as clients, so scheduled jobs run headless and all I/O
  passes through the policy gate.
- **Provider-agnostic brain** with model routing; saver mode routes to a cheap
  model and a single agent, normal mode can fan out to sub-agents.
- **Question box** (`ask_user`) is a first-class reasoning tool.
- **Self-modification** is the highest tier: diff review → accept-risk →
  rebuild → self-test → snapshot → hot-restart → auto-rollback.

## Build
```
go build ./...
```
