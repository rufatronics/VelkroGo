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
	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/scheduler"
	"github.com/rufatronics/velkrogo/internal/tools"
	"github.com/rufatronics/velkrogo/internal/worlds/coder"

	// Register provider factories via init().
	_ "github.com/rufatronics/velkrogo/internal/provider/anthropic"
	_ "github.com/rufatronics/velkrogo/internal/provider/gemini"
	_ "github.com/rufatronics/velkrogo/internal/provider/openaicompat"
)

func main() {
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
		tools.WebSearch{}, tools.FetchPage{},
		tools.RunShell{},
	}
	for _, t := range coder.AllCoderTools() {
		allTools = append(allTools, t)
	}
	for _, t := range allTools {
		if err := reg.Register(t); err != nil {
			log.Printf("register tool %s: %v", t.Name(), err)
		}
	}

	// Event channel.
	events := make(chan orchestrator.Event, 128)

	// Engine.
	engine := &orchestrator.Engine{
		Provider: prov,
		Model:    activeModel,
		Registry: reg,
		Policy:   policy.NewBasic(),
		World:    registry.WorldShared,
		Events:   events,
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

	log.Printf("velkrod: listening on http://%s  (web GUI: http://%s)", actual, actual)
	log.Printf("velkrod: database at %s", dbPath)

	<-ctx.Done()
	log.Println("velkrod: shutting down")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
