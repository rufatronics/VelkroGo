# How to Use VelkroGo

VelkroGo is a self-hosted AI agent that runs on your machine. It can write code, manage git repos, run shell commands, search the web, and automate tasks — all with your approval before anything sensitive happens.

---

## Installation

### Linux
```bash
# Download the GUI app
chmod +x velkroapp-linux-amd64
./velkroapp-linux-amd64
```

### Windows
Double-click `velkroapp-windows-amd64.exe`.

### Build from source
```bash
git clone https://github.com/rufatronics/VelkroGo
cd VelkroGo

# GUI (requires gcc + OpenGL headers)
# Linux: sudo apt-get install gcc libgl1-mesa-dev xorg-dev
go build ./cmd/velkroapp

# TUI (no extra deps)
go build ./cmd/velkro

# Headless daemon
go build ./cmd/velkrod
```

---

## First Run

On first launch a setup dialog appears. Pick your AI provider:

| Provider | Where to get a key |
|---|---|
| Anthropic (Claude) | console.anthropic.com |
| OpenAI | platform.openai.com |
| Google Gemini | aistudio.google.com |
| DeepSeek | platform.deepseek.com |
| Groq | console.groq.com |
| Mistral | console.mistral.ai |
| xAI (Grok) | console.x.ai |
| Ollama | Free — install from ollama.com, no key needed |
| LM Studio | Free — install from lmstudio.ai, no key needed |

**Tip:** Set your key as an environment variable instead of pasting it into the app:
```bash
export ANTHROPIC_API_KEY=sk-ant-...
./velkroapp-linux-amd64
```

---

## The Interface

```
┌────────────────────────────────────────────┐
│ ⚡ VelkroGo          [Settings]  [About]   │
├──────────────────────┬─────────────────────┤
│                      │  Plan               │
│  Chat transcript     │  ○ Step 1           │
│                      │  ▶ Step 2 (active)  │
│  You: fix the bug    │  ✓ Step 3           │
│  Agent: ...          │                     │
│  Tool: git_status    ├─────────────────────┤
│                      │  Provider · Model   │
│                      │  0 / 0 tok          │
├──────────────────────┴─────────────────────┤
│  [Type a task…                    ] [Send] │
│  Normal mode   [💰 Saver]   [⏹ Stop]      │
└────────────────────────────────────────────┘
```

- **Chat panel** — your conversation with the agent.
- **Plan panel** — the agent outlines numbered steps before it acts (like Manus). Watch it work step by step.
- **Send** — Ctrl+Enter or click. The agent plans, then executes.
- **Stop** — cancel the current task at any time.
- **Saver mode** — uses a cheaper model and minimal prompts to reduce API costs.

---

## Giving Tasks

Just type in plain English:

```
Fix the failing tests in ~/projects/myapp
```
```
Search the web for the latest Rust async patterns and summarise them
```
```
Clone github.com/me/repo, add a README, commit and push
```
```
Every morning at 9am, check my git repos for new PRs and summarise them
```

---

## Approval Gate

Before anything consequential runs, you'll see a popup:

```
⚠ Approval Required
Tool: run_shell   Risk tier: T3

sh -c "npm run build"

[Allow once]  [Allow for session]  [Deny]
```

- **Allow once** — runs this one action, asks again next time.
- **Allow for session** — never asks for this tool again until you restart.
- **Deny** — blocks the action; the agent notes it and adjusts.

### Risk tiers
| Tier | Examples | Default behaviour |
|---|---|---|
| T0 Read-only | web search, read file, git log | Runs automatically |
| T1 Local write | write file in workspace, git commit | Asks once |
| T2 External | git push, HTTP POST | Always asks + shows preview |
| T3 Device control | run shell, install software | Always asks |
| T4 Self-modify | edit VelkroGo's own code | Requires explicit risk acceptance |

---

## Question Box

When the agent is uncertain it pauses and asks before guessing:

```
Question: Which branch should I push to?

○ main
○ dev
○ Create a new branch

[Or type a custom answer…]   [Submit]
```

This prevents it from taking the wrong action on an ambiguous request.

---

## Settings — Managing Providers

Click **Settings** in the toolbar:
- See all your configured providers.
- **Add Provider** — pick from 16 presets or enter a custom URL.
- **Test Connection** — verifies your key works before saving.
- **Set as Default** — switches the active provider.
- **Remove** — deletes a provider.

You can have multiple providers and switch between them. For example: use Ollama (free, local) for quick tasks and Anthropic Claude for complex ones.

---

## Scheduler

The headless daemon (`velkrod`) supports scheduled jobs:

```bash
./velkrod &   # run in background
```

Use the REST API or add jobs directly to the state database. Supported schedule formats:
```
every 15m              # every 15 minutes
every 2h               # every 2 hours
0 9 * * 1-5            # 9am weekdays (cron)
once:2026-07-01T09:00:00Z   # one-shot on a specific date
```

Scheduled jobs still run through the approval gate — if a T2+ action comes up and no pre-grant exists, the job pauses and notifies you.

---

## TUI (Terminal)

For servers or people who prefer the terminal:

```bash
./velkro-linux-amd64
```

Same features as the GUI:
- Type tasks, press Enter to send.
- Plan panel shown above the chat.
- Approval prompts appear inline: `[y] allow once  [s] session  [n] deny`.
- Question box: type the number of your option.
- `Tab` opens the settings screen (navigate providers with ↑↓).
- `/saver` toggles cost mode.

---

## Headless Daemon

```bash
./velkrod-linux-amd64
```

Starts the API server on `localhost:7477`. Use this to:
- Run scheduled jobs without a UI open.
- Integrate with scripts via the REST API (`/api/run`, `/api/jobs`, `/api/audit`).
- Connect multiple clients (the GUI and TUI can both connect simultaneously).

---

## Cost Tips

- **Use saver mode** for routine tasks — it uses the cheapest model with a minimal prompt.
- **Ollama or LM Studio** — completely free, runs locally, no API costs at all. Quality is lower than hosted models but fine for many tasks.
- **Groq** — extremely fast and cheap with Llama models.
- The token counter in the bottom-right shows usage per session.

---

## Privacy

- Everything runs locally. No data is sent anywhere except to the AI provider you configure.
- Your API keys are stored in `~/.config/velkrogo/providers.json` (file permissions 600).
- Prefer the env-var approach (`ANTHROPIC_API_KEY=...`) to keep keys out of files entirely.
- The audit log at `~/.config/velkrogo/audit.db` records every action locally.

---

## Troubleshooting

**"No provider configured"**
Run the app — the setup dialog will appear automatically.

**"Connection failed" when testing provider**
- Check your API key is correct and has credits.
- For Ollama/LM Studio, make sure the local server is running (`ollama serve`).

**GUI doesn't start on Linux**
Install the graphics libraries:
```bash
sudo apt-get install libgl1-mesa-dev xorg-dev
```

**Agent keeps asking approval for the same tool**
Click **Allow for session** instead of Allow once, or pre-grant it in Settings.

**Build fails on Windows**
Make sure you have a C compiler (TDM-GCC or MinGW-w64) installed and in PATH. The GUI requires CGO.
