# VelkroGo — Project State

> **Last updated:** 2026-06-14 · **Version:** v1.1.0  
> **Repo:** https://github.com/rufatronics/VelkroGo  
> **Module:** `github.com/rufatronics/velkrogo`  
> **Go version:** 1.25.0

This document is the single source of truth for any AI builder or contributor picking up this project. It describes the full file tree, every package's purpose, what is built and working, what is not yet done, and exactly how everything fits together.

---

## What VelkroGo Is

A self-hosted AI agent for Windows and Linux. It can write code, manage git repos, search the web, control the desktop (mouse, keyboard, screenshots), integrate with Supabase and Vercel, and automate scheduled tasks — all with an approval gate before anything consequential happens.

Two "worlds":
- **World 1 — Coder:** git, GitHub API, build, test, file ops
- **World 2 — Operator:** screenshot, mouse, keyboard, open apps, shell

Three binaries, one shared engine (`internal/orchestrator`):
- `velkroapp` — Native desktop GUI (Fyne v2)
- `velkro` — Terminal TUI (Bubble Tea)
- `velkrod` — Headless daemon with REST + WebSocket API

---

## Full File Tree

```
VelkroGo/
├── .github/
│   └── workflows/
│       └── release.yml          # CI: builds all binaries + publishes GitHub Release on tag
├── .gitignore
├── ARCHITECTURE.md              # Full design doc (read first for deep context)
├── HOWTOUSE.md                  # Beginner user guide
├── PROJECT_STATE.md             # ← this file
├── README.md
├── ROADMAP.md                   # 10-phase build plan
├── go.mod
├── go.sum
│
├── cmd/
│   ├── velkro/                  # TUI binary (CGO_ENABLED=0, cross-compile)
│   │   ├── main.go              # First-run wizard, tool registration, engine init
│   │   └── tui.go               # Bubble Tea model, approval/question bridge, views
│   │
│   ├── velkroapp/               # Desktop GUI binary (CGO required, per-OS build)
│   │   └── main.go              # Fyne app, all UI, approval dialogs, question dialogs
│   │
│   └── velkrod/                 # Headless daemon (CGO_ENABLED=0, cross-compile)
│       └── main.go              # API server, scheduler, engine, tool registration
│
└── internal/
    ├── api/
    │   └── server.go            # HTTP+WebSocket server on localhost:7477
    ├── audit/
    │   └── audit.go             # Hash-chained SQLite audit log (SHA256 chain)
    ├── config/
    │   └── config.go            # App config helpers
    ├── integrations/
    │   ├── supabase/
    │   │   └── supabase.go      # Supabase REST tools (select/insert/update/delete/upload)
    │   └── vercel/
    │       └── vercel.go        # Vercel API tools (deploy/list/set-env)
    ├── memory/
    │   └── db.go                # SQLite state: sessions, messages, skills, jobs,
    │                            #   memory_facts, session_rules
    ├── orchestrator/
    │   ├── engine.go            # ReAct agent loop, dispatch, approval gate, built-ins
    │   ├── engine_test.go
    │   └── orchestrator.go      # CostMode, StepStatus, Step, Plan types
    ├── policy/
    │   ├── engine.go            # Grant system, T0-T4 evaluation
    │   ├── engine_test.go
    │   └── policy.go            # Engine interface, Grant, Request, Decision types
    ├── prompt/
    │   └── builder.go           # Layered system prompt builder
    ├── provider/
    │   ├── provider.go          # Provider interface + message/tool types
    │   ├── manager.go           # Entry, Store (JSON file), Build(), TestConnection()
    │   ├── preset/
    │   │   ├── presets.go       # 16 provider presets with defaults
    │   │   └── presets_test.go
    │   ├── anthropic/
    │   │   ├── anthropic.go     # Anthropic Messages API adapter
    │   │   └── init.go          # RegisterFactory via init()
    │   ├── gemini/
    │   │   ├── gemini.go        # Google Gemini native API adapter
    │   │   └── init.go
    │   └── openaicompat/
    │       ├── openaicompat.go  # OpenAI-compatible adapter (covers 12 providers)
    │       └── init.go
    ├── reasoning/
    │   └── question.go          # Question, Option, Answer, Asker interface
    ├── registry/
    │   ├── tool.go              # Tool interface, Tier (T0-T4), World constants, Result
    │   └── memory.go            # In-memory Registry implementation
    ├── scheduler/
    │   ├── scheduler.go         # Cron/"every Xm"/"once:RFC3339" scheduler
    │   └── scheduler_test.go
    ├── search/
    │   ├── duckduckgo.go        # DDG HTML scraper (no API key needed)
    │   └── fetch.go             # HTTP page fetcher + HTML→text stripper
    ├── soul/
    │   └── soul.go              # SOUL.md loader (~/.velkrogo/SOUL.md)
    ├── tools/
    │   ├── fs.go                # ReadFile (T0), ListDir (T0), WriteFile (T1)
    │   ├── fs_extra.go          # MakeDir, DeletePath, MovePath, CopyFile (all T1)
    │   ├── memory.go            # MemoryGet/Set/List/Delete tools (T0/T1)
    │   ├── search.go            # WebSearch (T0), FetchPage (T0)
    │   ├── shell.go             # RunShell (T3, bash/cmd, 30s timeout)
    │   └── skills.go            # SkillsList/Save/Invoke/Delete tools (T0/T1)
    └── worlds/
        ├── coder/
        │   ├── git.go           # GitStatus/Diff/Log (T0), Clone/Commit/Branch (T1), Push (T2)
        │   ├── github_api.go    # GitHub API: ListPRs (T0), CreatePR/Issue/MergePR (T2)
        │   └── run.go           # RunBuild (T1), RunTests (T1), AllCoderTools()
        └── operator/
            └── device.go        # Screenshot, MouseClick, MouseMove, KeyboardType,
                                 #   KeyPress, OpenApp (all T3)
                                 #   Linux: xdotool; Windows: PowerShell
```

---

## Package-by-Package Reference

### `internal/registry` — Tool contract
```go
type Tool interface {
    Name() string
    Description() string
    Tier() Tier           // T0=ReadOnly … T4=SelfModify
    World() World         // "shared" | "coder" | "operator"
    Schema() json.RawMessage
    Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

const (
    TierReadOnly        Tier = iota // T0
    TierReversibleLocal             // T1
    TierExternal                    // T2
    TierDeviceControl               // T3
    TierSelfModify                  // T4
)

const (
    WorldShared   World = "shared"
    WorldCoder    World = "coder"
    WorldOperator World = "operator"
)
```

### `internal/provider` — AI provider abstraction
```go
type Provider interface {
    Chat(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    Capabilities() Capabilities
}
// Three adapters: anthropic, gemini, openaicompat (covers 12+ providers)
// Factory pattern: each adapter registers itself via init() → RegisterFactory()
// Stored in: ~/.config/velkrogo/providers.json (mode 0600)
```

### `internal/orchestrator` — Agent loop
```go
type Engine struct {
    Provider     provider.Provider
    Model        string
    Registry     registry.Registry
    Policy       policy.Engine
    Asker        reasoning.Asker    // question box
    Approver     Approver           // approval gate
    Mode         CostMode           // Normal | Saver
    World        registry.World
    Events       chan<- Event        // streamed to frontend
    SystemPrompt string             // layered prompt (overrides default)
}
// Run(ctx, userInput) → ReAct loop, max 40 iterations
// Reset() → clears history and plan
// Built-in tools: set_plan, ask_user
// Events: "text" | "tool_start" | "tool_done" | "plan" | "usage" | "error"
```

### `internal/policy` — Approval gate
```go
// T0 → always Allow
// T1-T3 → Allow if grant exists, else Ask
// T4 → always Ask
// Grant: {Capability, Target, Scope, SessionOnly, ExpiresAt}
// AddGrant / RevokeGrant / RevokeAll
```

### `internal/memory` — SQLite state
Tables and their purpose:
```sql
sessions       -- named session records (id, title, created_at)
messages       -- conversation history per session
skills         -- named reusable prompt snippets
jobs           -- scheduled task definitions
memory_facts   -- persistent key-value facts (memory_set/get)
session_rules  -- per-session rule strings
```
DB path: `~/.config/velkrogo/state.db`

### `internal/prompt` — Layered system prompt
```
Layer 0: SOUL.md content         (~/.velkrogo/SOUL.md)
Layer 1: Session rules           (session_rules table)
Layer 2: Memory facts            (memory_facts table)
Layer 3: Available skills        (skills table — names + descriptions)
Layer 4: Tool list               (from Registry.Enabled(world, saverMode))
Layer 5: Mode hint               (normal instructions vs saver brevity hint)
```

### `internal/soul` — Identity file
- Loads `~/.velkrogo/SOUL.md`
- Creates default file on first run
- Content is injected as Layer 0 of every system prompt

### `internal/api` — Daemon REST API
```
POST /api/run                → run a prompt
GET  /api/jobs               → list scheduled jobs
POST /api/jobs               → create a job
DELETE /api/jobs/{id}        → delete a job
GET  /api/providers          → list providers
POST /api/providers/test     → test a provider
POST /api/providers/default  → set default
GET  /api/sessions           → list sessions
GET  /api/audit              → recent audit log
POST /api/approve            → approve a pending tool call
POST /api/answer             → answer a pending question
POST /api/mode               → set cost mode
POST /api/kill               → cancel current task
ws:  /ws                     → real-time event stream
```

### `internal/scheduler` — Job scheduler
Schedule formats supported:
```
every 15m               → every 15 minutes
every 2h                → every 2 hours
0 9 * * 1-5             → standard 5-field cron
once:2026-07-01T09:00Z  → one-shot at RFC3339 time
```
Polls every 30 seconds. Jobs run through the full approval gate.

---

## All 38 Tools

### Shared world (available in all modes)
| Tool | Tier | Description |
|------|------|-------------|
| `read_file` | T0 | Read file contents |
| `list_dir` | T0 | List directory |
| `write_file` | T1 | Write/overwrite file |
| `make_dir` | T1 | Create directory tree |
| `delete_path` | T1 | Delete file or directory |
| `move_path` | T1 | Move/rename |
| `copy_file` | T1 | Copy file |
| `web_search` | T0 | DuckDuckGo search (no key) |
| `fetch_page` | T0 | Download + parse web page |
| `run_shell` | T3 | bash/PowerShell command (30s) |
| `memory_get` | T0 | Recall a fact by key |
| `memory_set` | T1 | Store a persistent fact |
| `memory_list` | T0 | List all remembered facts |
| `memory_delete` | T1 | Forget a fact |
| `skills_list` | T0 | List saved skills |
| `skills_save` | T1 | Save a reusable prompt |
| `invoke_skill` | T0 | Run a skill by name |
| `skills_delete` | T1 | Delete a skill |
| `supabase_select` | T0 | Query Supabase table |
| `supabase_insert` | T1 | Insert row |
| `supabase_update` | T1 | Update rows |
| `supabase_delete` | T1 | Delete rows |
| `supabase_storage_upload` | T2 | Upload file to bucket |
| `vercel_list_deployments` | T0 | List deployments |
| `vercel_deploy` | T2 | Trigger deployment |
| `vercel_set_env` | T2 | Set env var on project |

### Coder world (World 1)
| Tool | Tier | Description |
|------|------|-------------|
| `git_status` | T0 | Show changed files |
| `git_diff` | T0 | Show diff |
| `git_log` | T0 | Show commit history |
| `git_clone` | T1 | Clone a repo |
| `git_commit` | T1 | Stage all + commit |
| `git_create_branch` | T1 | Create and switch branch |
| `git_push` | T2 | Push to remote |
| `run_build` | T1 | Run build command |
| `run_tests` | T1 | Run test suite |
| `github_list_prs` | T0 | List PRs (needs `GITHUB_TOKEN`) |
| `github_create_pr` | T2 | Open pull request |
| `github_create_issue` | T2 | Create issue |
| `github_merge_pr` | T2 | Merge PR |

### Operator world (World 2)
| Tool | Tier | Description |
|------|------|-------------|
| `screenshot` | T3 | Screen capture → base64 PNG |
| `mouse_click` | T3 | Click at (x, y) |
| `mouse_move` | T3 | Move cursor |
| `keyboard_type` | T3 | Type text |
| `key_press` | T3 | Press key combo (ctrl+c etc.) |
| `open_app` | T3 | Launch application |

Device control platform notes:
- **Linux:** requires `xdotool` (`sudo apt install xdotool`) for mouse/keyboard; `scrot` or `gnome-screenshot` for screenshots
- **Windows:** uses PowerShell `SendKeys` / `mouse_event` / `Start-Process`

---

## Environment Variables

| Variable | Used by | Purpose |
|----------|---------|---------|
| `ANTHROPIC_API_KEY` | anthropic adapter | Anthropic Claude key |
| `OPENAI_API_KEY` | openaicompat adapter | OpenAI key |
| `GEMINI_API_KEY` | gemini adapter | Google Gemini key |
| `GITHUB_TOKEN` or `GH_TOKEN` | github_api tools | GitHub personal access token |
| `SUPABASE_URL` | supabase tools | Project URL (https://x.supabase.co) |
| `SUPABASE_SERVICE_KEY` | supabase tools | Service-role key |
| `SUPABASE_ANON_KEY` | supabase tools | Anon key (fallback) |
| `VERCEL_TOKEN` | vercel tools | Vercel API token |
| `VELKRO_ADDR` | velkrod | Bind address (default `127.0.0.1:7477`) |
| `VELKRO_NO_COLOR` | velkro TUI | Disable all colour (PowerShell) |
| `NO_COLOR` | velkro TUI | Standard no-colour env var |

---

## 16 Supported AI Providers

| ID | Name | Kind | Key env var |
|----|------|------|-------------|
| `anthropic` | Anthropic Claude | `anthropic` | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI GPT | `openai-compatible` | `OPENAI_API_KEY` |
| `gemini` | Google Gemini | `gemini` | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `openai-compatible` | `DEEPSEEK_API_KEY` |
| `groq` | Groq | `openai-compatible` | `GROQ_API_KEY` |
| `mistral` | Mistral AI | `openai-compatible` | `MISTRAL_API_KEY` |
| `xai` | xAI (Grok) | `openai-compatible` | `XAI_API_KEY` |
| `together` | Together AI | `openai-compatible` | `TOGETHER_API_KEY` |
| `perplexity` | Perplexity AI | `openai-compatible` | `PERPLEXITY_API_KEY` |
| `cohere` | Cohere | `openai-compatible` | `COHERE_API_KEY` |
| `openrouter` | OpenRouter | `openai-compatible` | `OPENROUTER_API_KEY` |
| `fireworks` | Fireworks AI | `openai-compatible` | `FIREWORKS_API_KEY` |
| `cerebras` | Cerebras | `openai-compatible` | `CEREBRAS_API_KEY` |
| `ollama` | Ollama (local, free) | `openai-compatible` | none |
| `lmstudio` | LM Studio (local, free) | `openai-compatible` | none |
| `custom` | Custom endpoint | `openai-compatible` | optional |

---

## Data Storage

All data lives locally. Nothing sent to the cloud except prompts to the AI provider.

| Store | Path | Notes |
|-------|------|-------|
| State DB | `~/.config/velkrogo/state.db` | SQLite via modernc.org/sqlite (pure Go) |
| Audit log | `~/.config/velkrogo/audit.db` | SHA256 hash-chained entries |
| Providers | `~/.config/velkrogo/providers.json` | mode 0600, API keys stored here |
| Identity | `~/.velkrogo/SOUL.md` | User-editable agent personality |

---

## Build System

### Go version
Go 1.25.0

### Dependencies (direct)
```
fyne.io/fyne/v2 v2.7.4           GUI (CGO required)
github.com/charmbracelet/bubbletea v1.3.10   TUI
github.com/charmbracelet/lipgloss v1.1.0     TUI styling
github.com/gorilla/websocket v1.5.3           WebSocket API
golang.org/x/net v0.56.0                     HTML parsing
modernc.org/sqlite v1.52.0                   Pure-Go SQLite
```

### CGO rules
- `velkrod` and `velkro`: `CGO_ENABLED=0`, cross-compile from Linux
- `velkroapp`: CGO required for Fyne OpenGL — must build natively per OS

### GitHub Actions (`.github/workflows/release.yml`)
Triggered by: `push: tags: v*` or `workflow_dispatch`

| Job | Runner | Output |
|-----|--------|--------|
| `build-cli` matrix (linux-amd64, linux-arm64, windows-amd64) | ubuntu-latest | `velkrod-*`, `velkro-*` |
| `build-gui-linux` | ubuntu-latest + libgl1-mesa-dev xorg-dev | `velkroapp-linux-amd64` |
| `build-gui-windows` | windows-latest | `velkroapp-windows-amd64.exe` |
| `release` | ubuntu-latest | Publishes GitHub Release with all artifacts |

Release artifacts:
```
velkroapp-linux-amd64
velkroapp-windows-amd64.exe
velkro-linux-amd64
velkro-linux-arm64
velkro-windows-amd64.exe
velkrod-linux-amd64
velkrod-linux-arm64
velkrod-windows-amd64.exe
```

---

## TUI Behaviour (`velkro`)

### Entry points
- `main()` → first-run wizard if no providers → `runTUI(engine, events, store)`
- First-run: interactive wizard picks from 16 providers, stores to JSON

### States
```
stateInput    → user is typing
stateBusy     → engine.Run() goroutine is active
stateApproval → waiting for y/s/n on a tool call
stateQuestion → waiting for user answer to ask_user
```

### Slash commands (handled locally, never sent to AI)
```
help / /help / /?   → print full help inline
/saver              → toggle cost mode
/new                → engine.Reset(), clear plan + tokens
/sessions           → show session info
```

### Key bindings
```
Enter    → send message (stateInput)
Esc      → cancel running task (stateBusy) / deny approval (stateApproval)
Tab      → toggle chat ↔ settings view
Ctrl+C   → cancel + quit
y        → allow once (stateApproval)
s        → allow for session (stateApproval)
n        → deny (stateApproval)
1-9      → pick question option (stateQuestion)
[text]   → custom answer (stateQuestion, type + Enter)
↑↓       → navigate providers (viewSettings)
Enter    → set default provider (viewSettings)
Del      → remove provider (viewSettings)
```

### No-colour mode
Set `VELKRO_NO_COLOR=1` or `NO_COLOR=1` — strips all lipgloss styles, safe for PowerShell.

---

## GUI Behaviour (`velkroapp`)

### Layout
```
toolbar [Settings] [Help/About]
─────────────────────────────────────
chat list (scrollable)  │  plan list
                        │  (steps)
─────────────────────────────────────
[input entry — multi-line]  [Send] [Stop] [💰 Saver]
status label                token counter  mode label
```

### Key facts
- `widget.List` with `fyne.TextWrapWord` for chat messages
- Approval: `dialog.ShowCustom` popup with Allow Once / Session / Deny buttons
- Question: `dialog.ShowCustom` popup with radio options + custom text entry
- `driver/desktop` import removed (was causing build failures)
- DB stored on `VelkroApp` struct, closed via `win.SetOnClosed`
- Help/About button opens full scrollable feature guide in a dialog

### Fyne build requirement
Linux: `sudo apt-get install gcc libgl1-mesa-dev xorg-dev libxrandr-dev libxcursor-dev libxi-dev libxinerama-dev`  
Windows: TDM-GCC or MinGW-w64 (CGO)

---

## Daemon Behaviour (`velkrod`)

- Loads provider store, builds engine, opens SQLite DB
- Sets `tools.MemoryStore = db` and `tools.SkillsStore = db`
- Loads SOUL.md, queries memory facts + skills, builds layered system prompt
- Starts scheduler (polls every 30s)
- Starts HTTP+WS server on `VELKRO_ADDR` (default `127.0.0.1:7477`)
- Approval gate: for scheduled jobs, pauses at T2+ and waits for `POST /api/approve`
- `--help` / `-h` prints full daemon guide

---

## What Is DONE ✅

- [x] All three binaries build and run
- [x] 16 AI providers (3 adapters: anthropic, gemini, openaicompat)
- [x] ReAct agent loop with set_plan and ask_user built-ins
- [x] T0–T4 policy gate with grant system (once / session)
- [x] 38 registered tools across 3 worlds
- [x] File ops: read, write, list, mkdir, delete, move, copy
- [x] Web: DuckDuckGo search, page fetch + HTML→text
- [x] Shell: run_shell (bash/PowerShell)
- [x] Git: status, diff, log, clone, commit, branch, push
- [x] GitHub API: list PRs, create PR, create issue, merge PR
- [x] Build/test tools
- [x] Supabase: select, insert, update, delete, storage upload
- [x] Vercel: list deployments, deploy, set env vars
- [x] Device control: screenshot, mouse click/move, keyboard type/press, open app
- [x] SOUL.md identity system (auto-creates default)
- [x] Layered prompt: SOUL → rules → memory → skills → tools → mode
- [x] Memory tools: set/get/list/delete (persists in SQLite)
- [x] Skills tools: save/list/invoke/delete
- [x] SQLite DB with sessions, messages, skills, jobs, memory_facts, session_rules
- [x] Hash-chained audit log
- [x] Scheduler: cron, every Xm/h, once:RFC3339
- [x] REST + WebSocket API (8 endpoints + /ws)
- [x] Bubble Tea TUI with full approval/question flow
- [x] TUI slash commands: /help, /new, /saver, /sessions
- [x] Escape cancels running task (no goroutine leak)
- [x] NO_COLOR support for PowerShell
- [x] Custom text input in question box
- [x] Engine.Reset() for /new
- [x] Fyne GUI with chat + plan panels
- [x] Fyne approval/question dialogs
- [x] Fyne settings (add/remove/test/set-default providers)
- [x] Built-in --help on all 3 binaries (full beginner guide)
- [x] GitHub Actions: build-cli, build-gui-linux, build-gui-windows, release
- [x] HOWTOUSE.md, ARCHITECTURE.md, ROADMAP.md

---

## What Is NOT Done Yet ❌

- [ ] **Parallel multi-step orchestration** — dependency graph between plan steps, parallel sub-agent execution, retry logic. Currently all steps run sequentially in the ReAct loop.
- [ ] **Session sidebar in GUI** — visual list of named sessions to switch between. DB tables exist, UI not built.
- [ ] **Memory/Skills panels in GUI** — panels to view/edit remembered facts and saved skills.
- [ ] **Per-session rules UI** — `session_rules` table exists in DB, `AddRule`/`ListRules`/`DeleteRule` methods exist, but not wired to engine prompt building or exposed in any UI.
- [ ] **`/sessions` interactive switch in TUI** — currently only prints info. Needs session selection flow.
- [ ] **macOS ARM64 build** — not in CI matrix. Would need `macos-latest` runner with CGO for GUI.
- [ ] **Saver mode model override** — cost mode changes prompt brevity but does not automatically switch to a cheaper model variant.
- [ ] **Web GUI** — there was an earlier HTML/JS web UI; it was replaced by the Fyne native app. If a web client is wanted, the `/ws` endpoint is ready to stream events.
- [ ] **Multi-session engine** — the daemon holds one engine. Supporting truly independent parallel sessions would require an engine pool.
- [ ] **Agent self-modification (T4)** — tier defined, policy asks, but no actual T4 tools implemented.
- [ ] **Streaming responses** — provider adapters return full completion; streaming delta display is partially stubbed but not surfaced in TUI/GUI.

---

## Key Conventions for Future Contributors

### Adding a new tool
1. Create a struct implementing `registry.Tool` in the appropriate package
2. Set `Tier()` honestly — T0 = read-only, T1 = local write, T2 = external, T3 = device, T4 = self-modify
3. Set `World()` to `WorldShared`, `WorldCoder`, or `WorldOperator`
4. Write a JSON Schema in `Schema()` — the provider uses this for tool calling
5. Register in all three `cmd/*/main.go` allTools slices
6. No CGO — all tools must be pure Go (except the Fyne app itself)

### Adding a new provider
1. Create package under `internal/provider/<name>/`
2. Implement `provider.Provider` interface
3. Call `provider.RegisterFactory(kind, factoryFunc)` in an `init()` function
4. Import the package with `_` in all three `cmd/*/main.go` files
5. Add a preset to `internal/provider/preset/presets.go`
6. Add to the first-run wizard list in `cmd/velkro/main.go`

### Commit style
- Author: `rufatronics <ahmadgadamu@gmail.com>`
- No session URLs in commit messages
- No "Phase X", "skeleton", or AI-generated boilerplate in comments

---

## Releases

| Tag | Date | Notes |
|-----|------|-------|
| v1.0.0 | 2026-06-14 | Initial release: all providers, agent loop, GUI, TUI, daemon |
| v1.0.1 | 2026-06-14 | Fyne GUI + HOWTOUSE.md |
| v1.1.0 | 2026-06-14 | Full feature set: memory/skills/SOUL.md/device control/GitHub/Supabase/Vercel/help system |
