// Command velkrod is the VelkroGo headless daemon: runs the agent loop,
// scheduler, and state in the background and exposes a local JSON/WebSocket
// API on localhost:7477 for scripting and scheduled jobs.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rufatronics/velkrogo/internal/api"
	"github.com/rufatronics/velkrogo/internal/audit"
	"github.com/rufatronics/velkrogo/internal/integrations/supabase"
	"github.com/rufatronics/velkrogo/internal/integrations/vercel"
	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/prompt"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/scheduler"
	"github.com/rufatronics/velkrogo/internal/soul"
	"github.com/rufatronics/velkrogo/internal/tools"
	"github.com/rufatronics/velkrogo/internal/worlds/coder"
	"github.com/rufatronics/velkrogo/internal/worlds/operator"

	// Register provider factories via init().
	_ "github.com/rufatronics/velkrogo/internal/provider/anthropic"
	_ "github.com/rufatronics/velkrogo/internal/provider/gemini"
	_ "github.com/rufatronics/velkrogo/internal/provider/openaicompat"
)

var version = "dev"

const helpText = `
velkrod — VelkroGo headless daemon

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  WHAT IS VELKROD?
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  velkrod is the background engine for VelkroGo. It runs the AI
  agent, scheduler, and database silently in the background and
  exposes an API on localhost:7477.

  Use it when you want to:
  • Run scheduled tasks without a UI open (e.g. every morning at 9am)
  • Automate tasks from scripts using a simple REST API
  • Let the GUI and TUI share one persistent agent session

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  QUICK START (beginners)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  1. Make sure you have configured a provider first:
       ./velkro-linux-amd64          (run the TUI wizard once)
     OR set an env var:
       export ANTHROPIC_API_KEY=sk-ant-...

  2. Start the daemon in the background:
       ./velkrod-linux-amd64 &

  3. Send it a task from any terminal:
       curl -s -X POST http://localhost:7477/api/run \
         -H "Content-Type: application/json" \
         -d '{"prompt":"summarise ~/myproject git log"}'

  4. Stop it anytime:
       kill %1   (or Ctrl+C if running in foreground)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  REST API — all endpoints
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Run a task immediately:
    POST /api/run
    Body: {"prompt": "your task here"}

  Manage scheduled jobs:
    GET  /api/jobs              List all jobs
    POST /api/jobs              Create a new job
      Body: {"title":"Daily report","prompt":"...","schedule":"0 9 * * *"}
    DELETE /api/jobs/{id}       Delete a job

  Providers (which AI to use):
    GET  /api/providers         List configured providers
    POST /api/providers/test    Test a provider connection
    POST /api/providers/default Set the default provider

  Audit log (every action is recorded):
    GET /api/audit              View recent actions

  Sessions:
    GET /api/sessions           List named sessions

  Approval gate (for scheduled jobs):
    POST /api/approve           Approve a pending tool call
    POST /api/answer            Answer a pending question

  Live streaming (WebSocket):
    ws://localhost:7477/ws      Real-time event stream
    Events: text, tool_start, tool_done, plan, usage, error

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SCHEDULER — run tasks automatically
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Schedule formats:

    every 15m                   Every 15 minutes
    every 2h                    Every 2 hours
    every 30s                   Every 30 seconds
    0 9 * * 1-5                 9:00 am every weekday (standard cron)
    0 */4 * * *                 Every 4 hours
    once:2026-12-01T08:00:00Z   Run once at this exact UTC time

  Example — add a daily summary job:
    curl -X POST http://localhost:7477/api/jobs \
      -H "Content-Type: application/json" \
      -d '{
        "title": "Daily git summary",
        "prompt": "Check ~/myproject for new commits and summarise them",
        "schedule": "0 9 * * 1-5"
      }'

  Note: scheduled jobs still go through the approval gate.
  If the agent needs T2+ approval and no pre-grant exists,
  the job pauses and can be unblocked via POST /api/approve.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  FULL TOOL LIST — what the agent can do
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  FILE SYSTEM
    read_file, write_file, list_dir
    make_dir, delete_path, move_path, copy_file

  WEB
    web_search   (DuckDuckGo, no key needed)
    fetch_page   (download and read any URL)

  SHELL
    run_shell    (bash on Linux, cmd on Windows — T3, always asks)

  GIT
    git_status, git_diff, git_log
    git_clone, git_commit, git_create_branch, git_push

  GITHUB API  (set GITHUB_TOKEN)
    github_list_prs, github_create_pr
    github_create_issue, github_merge_pr

  BUILD & TEST
    run_build, run_tests

  SUPABASE  (set SUPABASE_URL + SUPABASE_SERVICE_KEY)
    supabase_select, supabase_insert, supabase_update
    supabase_delete, supabase_storage_upload

  VERCEL  (set VERCEL_TOKEN)
    vercel_list_deployments, vercel_deploy, vercel_set_env

  DEVICE CONTROL  (T3 — always asks approval)
    screenshot, mouse_click, mouse_move
    keyboard_type, key_press, open_app
    Linux: requires xdotool (apt install xdotool)
    Windows: uses PowerShell SendKeys / mouse_event

  MEMORY  (persists across sessions and restarts)
    memory_set, memory_get, memory_list, memory_delete

  SKILLS  (reusable named prompts)
    skills_save, skills_list, invoke_skill, skills_delete

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SOUL.md — agent identity file
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Edit ~/.velkrogo/SOUL.md to customise how the agent behaves.
  This file is injected at the start of every prompt. Examples:

    "Always respond in Spanish."
    "Never delete files without asking, even at T1."
    "This agent manages the staging environment at api.myapp.com."

  The file is created automatically with sensible defaults
  the first time velkrod starts.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  LAYERED PROMPT SYSTEM
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Every request builds a system prompt from these layers (in order):

  Layer 0  SOUL.md identity        (~/.velkrogo/SOUL.md)
  Layer 1  Session rules           (per-session restrictions)
  Layer 2  Remembered facts        (from memory_set)
  Layer 3  Available skills        (from skills_save)
  Layer 4  Tool list               (enabled tools for this world/mode)
  Layer 5  Mode hint               (normal vs cost-saver instructions)

  This means the agent always "knows" your preferences and
  remembered facts without you having to repeat them each time.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ENVIRONMENT VARIABLES
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  VELKRO_ADDR          Bind address (default: 127.0.0.1:7477)
                       Change to 0.0.0.0:7477 only on trusted networks
  ANTHROPIC_API_KEY    Anthropic Claude key
  OPENAI_API_KEY       OpenAI key
  GEMINI_API_KEY       Google Gemini key
  GITHUB_TOKEN         GitHub personal access token
  GH_TOKEN             Alternative GitHub token env var name
  SUPABASE_URL         Your Supabase project URL
  SUPABASE_SERVICE_KEY Supabase service-role key
  SUPABASE_ANON_KEY    Supabase anon key (fallback)
  VERCEL_TOKEN         Vercel API token

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  WHERE YOUR DATA LIVES
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Database       ~/.config/velkrogo/state.db    (sessions, jobs, memory)
  Audit log      ~/.config/velkrogo/audit.db    (every action recorded)
  Providers      ~/.config/velkrogo/providers.json  (mode 600)
  Identity       ~/.velkrogo/SOUL.md

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SECURITY NOTES
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  • The daemon binds to 127.0.0.1 only by default.
    Never expose port 7477 to the internet without auth.
  • API keys are stored with 600 permissions (owner-only).
  • Setting keys via environment variables is safer than
    storing them in config files.
  • Every tool call is recorded in the audit log.

Full docs: https://github.com/rufatronics/VelkroGo
`

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Print(helpText)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("velkrod", version)
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// State DB.
	dbPath, err := memory.DefaultPath()
	if err != nil {
		log.Fatal("db path:", err)
	}
	db, err := memory.Open(dbPath)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer db.Close()

	// Wire memory/skills stores before registering tools.
	tools.MemoryStore = db
	tools.SkillsStore = db

	// Audit log.
	alogPath := filepath.Join(filepath.Dir(dbPath), "audit.db")
	alog, err := audit.Open(alogPath)
	if err != nil {
		log.Fatal("open audit log:", err)
	}
	defer alog.Close()

	// Provider store.
	store, err := provider.LoadStore()
	if err != nil {
		log.Fatal("load provider store:", err)
	}

	// Build the active provider from the default entry.
	def := store.Default()
	if def == nil {
		fmt.Fprintln(os.Stderr, "No provider configured. Run `velkro` to set one up, or add a provider via the web GUI.")
	}

	var prov provider.Provider
	var activeModel string
	if def != nil {
		prov, err = provider.Build(*def)
		if err != nil {
			log.Fatal("build provider:", err)
		}
		activeModel = def.Model
	}

	// Tool registry.
	reg := registry.NewMemory()
	allTools := []registry.Tool{
		tools.ReadFile{}, tools.ListDir{}, tools.WriteFile{},
		tools.MakeDir{}, tools.DeletePath{}, tools.MovePath{}, tools.CopyFile{},
		tools.WebSearch{}, tools.FetchPage{},
		tools.RunShell{},
		tools.MemoryGet{}, tools.MemorySet{}, tools.MemoryList{}, tools.MemoryDelete{},
		tools.SkillsList{}, tools.SkillsSave{}, tools.SkillsInvoke{}, tools.SkillsDelete{},
	}
	for _, t := range coder.AllCoderTools() {
		allTools = append(allTools, t)
	}
	for _, t := range coder.AllGitHubAPITools() {
		allTools = append(allTools, t)
	}
	for _, t := range supabase.AllSupabaseTools() {
		allTools = append(allTools, t)
	}
	for _, t := range vercel.AllVercelTools() {
		allTools = append(allTools, t)
	}
	for _, t := range operator.AllOperatorTools() {
		allTools = append(allTools, t)
	}
	for _, t := range allTools {
		if err := reg.Register(t); err != nil {
			log.Printf("register tool %s: %v", t.Name(), err)
		}
	}

	// Build layered system prompt: SOUL → memory → skills → instructions.
	soulContent := soul.Load()
	facts, _ := db.ListMemory()
	skills, _ := db.ListSkills()
	sysPrompt := prompt.Build(prompt.Config{
		Soul:   soulContent,
		Facts:  facts,
		Skills: skills,
	})

	// Event channel.
	events := make(chan orchestrator.Event, 128)

	// Engine.
	engine := &orchestrator.Engine{
		Provider:     prov,
		Model:        activeModel,
		Registry:     reg,
		Policy:       policy.NewBasic(),
		World:        registry.WorldShared,
		Events:       events,
		SystemPrompt: sysPrompt,
	}

	// Scheduler.
	sched := scheduler.New(db, func(ctx context.Context, job memory.Job) {
		log.Printf("scheduler: running job %q", job.Title)
		alog.Append(audit.KindSchedule, "scheduled", job.ID, job)
		if err := engine.Run(ctx, job.Prompt); err != nil {
			log.Printf("scheduler: job %q error: %v", job.Title, err)
		}
	})
	sched.Start(ctx)
	defer sched.Stop()

	// API server.
	addr := envOr("VELKRO_ADDR", "127.0.0.1:7477")
	srv := api.New(addr, engine, db, alog, store, sched, events)
	actual, err := srv.Start(ctx)
	if err != nil {
		log.Fatal("start API server:", err)
	}

	log.Printf("velkrod %s: listening on http://%s", version, actual)
	log.Printf("velkrod: database at %s", dbPath)
	log.Printf("velkrod: identity file at ~/.velkrogo/SOUL.md")

	<-ctx.Done()
	log.Println("velkrod: shutting down")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
