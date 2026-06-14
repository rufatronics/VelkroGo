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

const helpText = `velkrod — VelkroGo headless daemon

USAGE:
  velkrod [--help]

WHAT IT DOES:
  Runs the AI agent loop, scheduler, and state database in the background.
  Exposes a local JSON/WebSocket API on localhost:7477 that the GUI and TUI
  connect to, and that you can call from your own scripts.

QUICK START:
  # Start the daemon in the background
  ./velkrod-linux-amd64 &

  # Run a one-shot task via the REST API
  curl -X POST http://localhost:7477/api/run \
    -H "Content-Type: application/json" \
    -d '{"prompt":"summarise the git log in ~/myproject"}'

  # List scheduled jobs
  curl http://localhost:7477/api/jobs

  # Watch live events over WebSocket
  wscat -c ws://localhost:7477/ws

API ENDPOINTS:
  POST /api/run                 Run a prompt immediately
  GET  /api/jobs                List scheduled jobs
  POST /api/jobs                Add a scheduled job
  GET  /api/audit               Recent audit log entries
  GET  /api/providers           List configured providers
  POST /api/providers/test      Test a provider connection
  POST /api/providers/default   Set the default provider

SCHEDULE FORMATS:
  every 15m                     Every 15 minutes
  every 2h                      Every 2 hours
  0 9 * * 1-5                   9 am on weekdays (cron)
  once:2026-07-01T09:00:00Z    Run once at this exact time

ENVIRONMENT VARIABLES:
  VELKRO_ADDR          Listen address (default 127.0.0.1:7477)
  ANTHROPIC_API_KEY    Anthropic key (recommended over pasting in wizard)
  OPENAI_API_KEY       OpenAI key
  GEMINI_API_KEY       Google Gemini key
  GITHUB_TOKEN         GitHub API key (for github_create_pr, github_list_prs, etc.)
  SUPABASE_URL         Your Supabase project URL
  SUPABASE_SERVICE_KEY Supabase service-role key
  VERCEL_TOKEN         Vercel API token

DATA:
  Database   ~/.config/velkrogo/state.db
  Audit log  ~/.config/velkrogo/audit.db
  Identity   ~/.velkrogo/SOUL.md  (edit to customise agent personality)

SECURITY:
  The daemon only binds to localhost by default. Never expose port 7477
  to the network without adding authentication.

See https://github.com/rufatronics/VelkroGo for full documentation.
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
