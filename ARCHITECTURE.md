# VelkroGo — Architecture & Design

> A self-hostable, cross-platform (Windows + Linux) AI agent written in Go that
> fuses a **coding agent** (à la OpenDevin) and a **general-purpose computer-use
> agent** (à la "OpenClaw"/Claude computer-use) behind one safety-first core.

This document is the design-first deliverable: it outlines the architecture and
the build steps before any feature code is written. Implementation follows the
phased [ROADMAP](./ROADMAP.md).

---

## 1. Product vision in one paragraph

VelkroGo is a single binary that runs locally on a user's machine. It exposes
**two "worlds"** that share one agent core:

- **World 1 — Coder.** Connects to GitHub/GitLab and other dev services, reads
  and writes code across repos, runs builds/tests, and ships full apps.
- **World 2 — Operator.** Drives the actual device — files, shell, apps,
  browser, network — to do everyday tasks.

Everything dangerous is gated behind **explicit, scoped user approval**. The
agent can **schedule** work ("do X on June 20", "every 15 min"), **search the
web** cheaply (DuckDuckGo scrape + page reading) or drive a **real browser with
human takeover**, **reason Claude-style** by surfacing a question box before it
proceeds when it's uncertain, run in a **money-saving mode**, and even
**improve its own code / add skills** — but only when the user accepts the risk.

---

## 2. Design principles (the "community standards" we hold to)

1. **Human-in-the-loop by default.** Side-effecting actions require approval.
   Approvals are *scoped* (this command, this session, this directory, always)
   and *revocable*.
2. **Capabilities, not vibes.** Every power the agent has is a registered
   *capability* with an explicit risk tier. Nothing acts outside a granted
   capability.
3. **Auditable.** Every tool call, approval, file write, and network request is
   appended to a tamper-evident local log the user can inspect/export.
4. **Sandbox first.** Code execution and computer-use default to the most
   isolated backend available (container/VM); raw host access is opt-in.
5. **Provider-agnostic.** The brain is swappable. No business logic depends on a
   specific model vendor.
6. **Least context, lowest cost that works.** Prompts, memory, and the number of
   concurrent sub-agents scale with the task and the selected cost mode.
7. **Local-first & portable.** Single Go binary, no mandatory cloud service,
   state in a local DB; same behavior on Windows and Linux.

---

## 3. Risk tiers (the backbone of approvals)

Every capability and tool declares a tier. The UI and policy engine treat them
differently.

| Tier | Examples | Default gate |
|------|----------|--------------|
| **T0 Read-only** | web search, read file, list repo, read PR | auto-allow (configurable) |
| **T1 Reversible local** | write file in workspace, git branch/commit (no push) | one-click approve, batchable |
| **T2 External / outward** | git push, open PR, send email, HTTP POST, browser form submit | per-action approval w/ preview |
| **T3 Device control** | run host shell, control mouse/keyboard, install software, registry/systemd edits | per-action + scope, sandbox preferred |
| **T4 Self-modification** | edit VelkroGo's own source, add/replace skills, rebuild & restart | explicit "I accept the risk" + diff review + auto-snapshot/rollback |

Cost mode and tier interact: even in money-saving mode, **T2+ always asks**.

---

## 4. High-level architecture

```
                         ┌──────────────────────────────────────────┐
                         │                Frontends                  │
                         │   GUI (lightweight)      TUI (Bubble Tea) │
                         └───────────────┬──────────────┬───────────┘
                                         │  local API (HTTP+WS / JSON-RPC)
                         ┌───────────────▼──────────────▼───────────┐
                         │                 Daemon (velkrod)           │
                         │                                            │
                         │  ┌──────────────┐   ┌───────────────────┐ │
                         │  │ Orchestrator │   │  Policy & Approval │ │
                         │  │ (agent loop) │◄─►│  Engine (gate)     │ │
                         │  └──────┬───────┘   └─────────┬─────────┘ │
                         │         │                     │           │
                         │  ┌──────▼───────┐   ┌─────────▼─────────┐ │
                         │  │ Reasoning /  │   │   Audit Log        │ │
                         │  │ Question box │   │   (append-only)    │ │
                         │  └──────┬───────┘   └───────────────────┘ │
                         │         │                                  │
                         │  ┌──────▼───────────────────────────────┐ │
                         │  │   Capability / Tool Registry          │ │
                         │  └─┬─────┬─────┬─────┬─────┬─────┬──────┘ │
                         │    │     │     │     │     │     │         │
                         │  Coder Oper. Search Browse Sched Skills    │
                         │  World1 World2                             │
                         │                                            │
                         │  ┌───────────────┐  ┌───────────────────┐ │
                         │  │ Provider layer │  │ Memory & State DB │ │
                         │  │ (LLM adapters) │  │ (SQLite)          │ │
                         │  └───────────────┘  └───────────────────┘ │
                         │  ┌───────────────────────────────────────┐│
                         │  │ Sandbox / Executor (container/VM/host) ││
                         │  └───────────────────────────────────────┘│
                         └────────────────────────────────────────────┘
```

**Why a daemon + thin frontends?** The GUI and TUI are just clients of one
long-running local daemon (`velkrod`). That lets scheduled jobs keep running
when no UI is open, lets GUI and TUI share one session, and keeps the agent
loop in one place. The frontends never talk to providers or the OS directly —
only through the daemon's local API, so the policy engine can mediate
*everything*.

---

## 5. Component breakdown

### 5.1 Orchestrator (the agent loop)
The core ReAct-style loop: build context → ask provider for next step → if it's
a tool call, route through the policy gate → execute → observe → repeat until
done or a question is needed. Responsibilities:

- Owns the **task/turn lifecycle** and the per-task plan (Manus-style explicit
  step outline the user can watch and edit).
- Decides **single-agent vs. multi-agent** execution based on cost mode and task
  complexity.
- Emits structured events (plan updates, tool calls, questions, results) over WS
  so GUI/TUI render live.

**Planner:** before executing, the orchestrator produces a visible numbered plan
(`todo list`). Each step has a status (pending/active/done/blocked). The user
can reorder, skip, or edit steps. This is the "outline steps first" behavior.

### 5.2 Reasoning & the Question Box (Claude-style)
A dedicated mechanism the model can invoke as a first-class tool: `ask_user`.
When the agent is **uncertain, faces an ambiguous fork, or is about to do
something with meaningful blast radius**, it pauses and renders a structured
**question box** (1–4 questions, each with 2–4 options + free-text "Other"),
exactly like Claude's clarifying-question UX. The loop blocks on the answer.

Triggers for auto-asking:
- Ambiguous requirement / multiple valid interpretations.
- About to take a T2+ action whose target is inferred, not stated.
- Conflicting evidence (e.g., a file contradicts the user's description).
- Self-modification or destructive operations (always ask).

### 5.3 Policy & Approval Engine
The single chokepoint for side effects. Inputs: the tool call + its declared
tier + current grant set + cost mode. Outputs: `allow` / `ask` / `deny`.

- **Grants** are scoped objects: `{capability, scope, expiry}` e.g. "allow
  `fs.write` under `~/proj` for this session", "always allow `web.search`".
- **Preview before approve:** for T2+ the user sees the exact command, diff,
  HTTP request, or browser action before confirming.
- **Kill switch / pause:** user can pause the agent or revoke all grants
  instantly.
- Policy is data (a rules file) so power users can pre-configure it.

### 5.4 Capability / Tool Registry
Tools register themselves with: name, JSON schema, risk tier, world tag
(coder/operator/shared), and an executor fn. The provider layer only ever sees
the schemas of *enabled* tools for the current world + cost mode (fewer tools =
cheaper, safer prompts). Skills (5.11) register here too.

### 5.5 Provider layer (AI providers + custom)
A narrow `Provider` interface so models are swappable and mixable:

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
    Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error)
    Capabilities() ProviderCaps // tools? vision? streaming? json mode?
}
```

- **Built-in adapters:** Anthropic Claude (default brain — current models:
  `claude-fable-5`, `claude-opus-4-8`, `claude-sonnet-4-6`,
  `claude-haiku-4-5-20251001`), OpenAI, Google Gemini, and any
  **OpenAI-compatible** endpoint (covers Ollama, LM Studio, OpenRouter, vLLM,
  most local servers).
- **Custom providers:** a config-driven generic adapter (base URL, auth header,
  request/response field mapping) so users add new vendors without code; plus a
  Go plugin / external-process option for truly custom logic.
- **Model routing:** roles map to models — e.g. a cheap "small" model for
  planning/summaries and a strong model for hard reasoning. Money-saving mode
  routes aggressively to the small model.

### 5.6 World 1 — Coder
Capabilities (selection):
- **VCS:** GitHub/GitLab via their APIs (clone, branch, commit, push, PRs,
  issues, reviews, CI status), plus generic `git`.
- **Workspace:** read/write project files in a sandboxed working dir.
- **Build/test/run:** language-aware runners executed in the sandbox; capture
  logs, iterate on failures.
- **Delivery:** scaffold full apps, produce diffs, open PRs, attach artifacts.

Coder runs against a **project workspace** (a checked-out repo dir) and prefers
the sandbox executor for all build/test/run steps.

### 5.7 World 2 — Operator (computer use)
Capabilities (all T3 unless noted):
- **Shell** in sandbox or host (host = approval each time).
- **Filesystem** beyond the workspace (T1/T2 depending on location).
- **GUI control:** screenshot + mouse/keyboard via a computer-use backend.
- **App & OS integration:** launch apps, clipboard, notifications; OS-specific
  adapters for Windows and Linux behind one interface.
- **Network/devices:** HTTP clients, and a pluggable connector model for
  "connect to virtually anything" (APIs, SSH, MQTT, etc.) — each connector is a
  capability with its own tier.

### 5.8 Search (lightweight, no browser required)
For the common "just look something up" case, avoid spinning a browser:
- **DuckDuckGo HTML/Lite scrape** for result lists (title, url, snippet).
- **Fetch & extract:** the agent can open a result URL and get cleaned readable
  text (readability-style extraction) to "read further."
- Politeness: rate limiting, caching, user-agent, robots-aware; easy to swap in
  a real search API later.
This is a T0 capability and works great in money-saving mode.

### 5.9 Browser control + human takeover (Manus-style)
When a task truly needs a browser (logins, JS-heavy sites, forms):
- Drive a real browser via CDP (Chrome DevTools Protocol) / Playwright-style
  control from Go.
- **Takeover handoff:** user clicks "Take over", the agent **pauses and yields
  the live browser**; the user does manual steps (e.g. solve a captcha, log in),
  then clicks "Resume" and the agent continues from the new page state.
- Every navigation/click/submit that's outward-facing is T2 and previewable.

### 5.10 Scheduler
Natural-language and explicit scheduling:
- "do this on 2026-06-20 09:00" → one-shot job.
- "every 15 min" / "every weekday" → recurring (cron-like).
- Jobs are persisted; the daemon runs them even with no UI open. Each run is a
  normal task subject to the same policy gate — **scheduled T2+ actions still
  require either a pre-granted scope or they pause and notify the user.**

### 5.11 Skills & self-modification (T4)
Two layers:
- **Skills (additive, safer):** self-contained units (a prompt + optional tools
  + metadata) the agent can author, install, and reuse — like Claude Code
  skills. Adding a skill is gated but lower-risk than touching core.
- **Self-improvement (core code):** the agent can read its *own* repo, propose
  diffs, and — only after the user toggles "accept self-modification risk" and
  reviews the diff — apply them, **rebuild**, run its **own test suite**,
  snapshot the previous binary/commit, and **hot-restart**. Automatic
  **rollback** if the new build fails health checks. This is the highest tier
  and always asks.

### 5.12 Memory & State
- **SQLite** (via a pure-Go driver for easy cross-compilation) for: sessions,
  task history, plans, audit log, grants, schedules, skill registry, provider
  config.
- **Working memory:** rolling context with summarization to control tokens.
- **Long-term memory:** optional embeddings store for recall across sessions
  (pluggable; off by default to stay lightweight).

### 5.13 Cost modes
- **Saver mode:** one agent at a time (no parallel sub-agents), minimal system
  prompt, reduced tool schema, aggressive context summarization, routes to the
  cheap model, fewer reasoning passes, search-over-browser preference.
- **Normal mode:** may spawn specialized sub-agents (planner/coder/reviewer),
  richer prompts and tool sets, stronger models, more verification passes.
The mode is a single switch that reconfigures the orchestrator + provider
routing + tool registry exposure.

---

## 6. Frontends

### 6.1 Lightweight GUI
- **Approach:** the daemon serves a small local web UI (HTML/JS, served on
  localhost) optionally wrapped as a desktop window. This keeps the binary light
  and cross-platform without a heavy native toolkit, and the same UI works in a
  browser tab if the user prefers.
- Shows: chat, the live **plan/step outline**, the **question box**, the
  **approval prompts with previews**, the live **browser pane** with takeover,
  audit log, schedules, provider/cost settings.

### 6.2 TUI
- **Bubble Tea**-based terminal UI for power users / headless servers: same
  events (chat, plan, questions, approvals) rendered in the terminal.

Both are thin clients over the daemon's WS/HTTP API, so they always agree.

---

## 7. Cross-platform (Windows + Linux)

- Pure-Go where possible; build tags (`_windows.go` / `_linux.go`) for OS
  specifics (shell invocation, GUI automation, autostart/service install,
  notifications, paths).
- Sandbox backend is pluggable: container (Docker/Podman) when present, else a
  restricted host-exec mode with explicit warnings.
- Single `go build` per OS; CI cross-compiles release binaries for both.

---

## 8. Security model (summary)

- Secrets (API keys, tokens) stored via OS keychain where available, else an
  encrypted local file; never logged.
- All side effects flow through the policy gate; T2+ previewed; T3/T4 sandboxed
  or explicitly accepted.
- Append-only, hash-chained audit log; exportable.
- Network egress allowlist option; per-capability on/off.
- Self-modification snapshots + auto-rollback; scheduled jobs can't silently
  escalate privileges.

---

## 9. Proposed repo layout

```
velkrogo/
  cmd/
    velkrod/        # the daemon
    velkro/         # CLI/TUI client
  internal/
    orchestrator/   # agent loop + planner
    reasoning/      # question box / ask_user
    policy/         # approval engine + grants
    audit/          # append-only log
    registry/       # capability/tool registry
    provider/       # LLM adapter interface + adapters
      anthropic/
      openai/       # also covers OpenAI-compatible/custom
      gemini/
      custom/       # config-driven generic adapter
    worlds/
      coder/        # World 1 tools (git, github, build/test)
      operator/     # World 2 tools (shell, fs, gui, devices)
    search/         # duckduckgo scrape + readability fetch
    browser/        # CDP control + takeover
    scheduler/      # cron/one-shot jobs
    skills/         # skill registry + self-modification
    sandbox/        # executor backends (container/host)
    memory/         # sqlite state + context mgmt
    api/            # local HTTP+WS server
    config/         # config + secrets
  web/              # lightweight GUI assets
  pkg/              # exported helpers (if any)
  ARCHITECTURE.md
  ROADMAP.md
```

---

## 10. Key decisions (confirmed with the user, 2026-06-12)

1. **GUI delivery:** local web UI served by the daemon (open in browser or a
   thin window). Lightest and identical on Windows/Linux.
2. **Default brain:** none — a **first-run wizard** lets the user pick a
   provider (Anthropic, OpenAI, or any OpenAI-compatible/custom endpoint) and
   configure model + key.
3. **Sandbox baseline:** **host execution by default**, gated by per-action
   approvals; container isolation is offered as an opt-in hardening when
   Docker/Podman is available.
4. **Computer-use backend** for World 2 GUI control per OS: still open;
   decided in Phase 5.
