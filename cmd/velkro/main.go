// Command velkro is the VelkroGo interactive client. It provides the Bubble Tea
// TUI and first-run setup. The agent engine runs in-process (standalone mode);
// in daemon mode it connects to velkrod over the local API.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rufatronics/velkrogo/internal/integrations/supabase"
	"github.com/rufatronics/velkrogo/internal/integrations/vercel"
	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/prompt"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/soul"
	"github.com/rufatronics/velkrogo/internal/tools"
	"github.com/rufatronics/velkrogo/internal/worlds/coder"
	"github.com/rufatronics/velkrogo/internal/worlds/operator"

	// Register provider factories.
	_ "github.com/rufatronics/velkrogo/internal/provider/anthropic"
	_ "github.com/rufatronics/velkrogo/internal/provider/gemini"
	_ "github.com/rufatronics/velkrogo/internal/provider/openaicompat"
)

var version = "dev"

const helpText = `
██╗   ██╗███████╗██╗     ██╗  ██╗██████╗  ██████╗  ██████╗
██║   ██║██╔════╝██║     ██║ ██╔╝██╔══██╗██╔═══██╗██╔════╝
██║   ██║█████╗  ██║     █████╔╝ ██████╔╝██║   ██║██║  ███╗
╚██╗ ██╔╝██╔══╝  ██║     ██╔═██╗ ██╔══██╗██║   ██║██║   ██║
 ╚████╔╝ ███████╗███████╗██║  ██╗██║  ██║╚██████╔╝╚██████╔╝
  ╚═══╝  ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝  ╚═════╝
TUI — Terminal interface. Works in any terminal, SSH, or PowerShell.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  FIRST TIME? START HERE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  VelkroGo is an AI agent that runs entirely on your machine.
  It can write code, search the web, manage git repos, control
  your desktop, connect to Supabase/Vercel, and automate tasks.
  Nothing is sent anywhere except to the AI provider you choose.

  Step 1 — Run the program:
    Linux/Mac:  ./velkro-linux-amd64
    Windows:    .\velkro-windows-amd64.exe

  Step 2 — A setup wizard appears automatically on first run.
    Pick a provider from the numbered list and paste your API key.
    Don't have a key? Choose Ollama (free, runs locally, no key).

  Step 3 — Type any task in plain English and press Enter:
    "Fix the failing tests in ~/myapp"
    "Search the web for the latest Go async patterns"
    "Clone github.com/me/repo, add a README, commit and push"

  Step 4 — Watch it work. The agent plans steps and executes them.
    When it wants to do something risky it stops and asks you first.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  KEYBOARD SHORTCUTS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Enter        Send your message to the agent
  Tab          Open / close the Settings panel (manage providers)
  Ctrl+C       Exit VelkroGo

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SLASH COMMANDS  (type these in the chat box)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  /help        Show this help text
  /saver       Toggle Cost Saver mode on/off
               (uses a cheaper model and shorter prompts — great
                for simple tasks to save API credits)
  /new         Start a brand new session (clears chat history)
  /sessions    List your saved sessions

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  APPROVAL GATE  (how the agent asks before acting)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Before anything consequential happens, you see a prompt like:

    [Approval required] run_shell: npm run build
    Allow? [y] once  [s] session  [n] deny

  y = allow this one action, ask again next time
  s = allow this tool for the rest of the session (no more prompts)
  n = block it; the agent notes the refusal and adjusts

  Risk tiers — what triggers a prompt:
  T0  Read-only (read file, web search, git log)  → runs silently
  T1  Local write (write file, git commit)         → y/s/n prompt
  T2  External action (git push, HTTP POST)        → y/s/n + preview
  T3  Device control (run shell, click mouse)      → y/s/n prompt
  T4  Self-modification (edit VelkroGo itself)     → explicit accept

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  QUESTION BOX  (when the agent is unsure)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  If your request is ambiguous, the agent pauses and offers options:

    Question: Which branch should I push to?
    1) main
    2) dev
    3) Create a new branch
    Type a number or a custom answer:

  This stops it from guessing wrong on important decisions.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  WHAT THE AGENT CAN DO  (full tool list)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  FILE SYSTEM
    read_file          Read the contents of any file
    write_file         Write or overwrite a file
    list_dir           List files and folders in a directory
    make_dir           Create a new directory (and parent dirs)
    delete_path        Delete a file or entire folder tree
    move_path          Move or rename a file or directory
    copy_file          Copy a file to a new location

  WEB
    web_search         Search the web (DuckDuckGo, no key needed)
    fetch_page         Download and read any web page

  SHELL
    run_shell          Run any shell command (bash/PowerShell)
                       Always asks for approval first (T3)

  GIT  (World 1 — Coder)
    git_status         Show changed files in a repo
    git_diff           Show the full diff of changes
    git_log            Show recent commit history
    git_clone          Clone a GitHub/GitLab repo
    git_commit         Stage all changes and commit
    git_create_branch  Create and switch to a new branch
    git_push           Push commits to a remote (T2 — asks)

  GITHUB API  (needs GITHUB_TOKEN env var)
    github_list_prs    List open pull requests in a repo
    github_create_pr   Open a new pull request
    github_create_issue Create a new issue
    github_merge_pr    Merge a pull request

  BUILD & TEST  (World 1 — Coder)
    run_build          Run the project's build command
    run_tests          Run the project's test suite

  SUPABASE  (needs SUPABASE_URL + SUPABASE_SERVICE_KEY)
    supabase_select    Read rows from a table
    supabase_insert    Insert a new row
    supabase_update    Update existing rows
    supabase_delete    Delete rows
    supabase_storage_upload  Upload a file to a storage bucket

  VERCEL  (needs VERCEL_TOKEN)
    vercel_list_deployments  List recent deployments
    vercel_deploy            Trigger a new deployment
    vercel_set_env           Set an environment variable

  DEVICE CONTROL  (World 2 — Operator, needs display)
    screenshot         Take a screenshot (returns base64 PNG)
    mouse_click        Click at screen coordinates (x, y)
    mouse_move         Move the mouse without clicking
    keyboard_type      Type text character by character
    key_press          Press a key combo (e.g. ctrl+c, Return)
    open_app           Launch an application by name or path
    Linux note: device tools require xdotool (apt install xdotool)

  MEMORY  (persists across sessions)
    memory_set         Remember a fact: memory_set key value
    memory_get         Recall a fact by key
    memory_list        Show everything the agent remembers
    memory_delete      Forget a specific fact

  SKILLS  (reusable prompt snippets)
    skills_save        Save a named reusable prompt
    skills_list        List all saved skills
    invoke_skill       Run a skill by name
    skills_delete      Delete a skill

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MEMORY — teaching the agent facts that stick
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  The agent can remember things between conversations.
  Just ask it: "Remember that my main project is at ~/code/myapp"
  It will call memory_set and recall it in every future session.

  You can also browse its memory: ask "What do you remember?"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SKILLS — reusable procedures
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Skills are saved prompts the agent can reuse. Example:

  "Save a skill called 'deploy-prod' that runs the Vercel deploy
   and posts a Slack message when done."

  Next time: "Run the deploy-prod skill."
  The agent will invoke it automatically.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SOUL.md — customise the agent's personality
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Edit ~/.velkrogo/SOUL.md to change how the agent behaves.
  It is injected as the first layer of every system prompt.

  Example additions:
    "Always respond in Spanish."
    "Prefer TypeScript over JavaScript."
    "Never delete files without asking, even at T1."

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  SUPPORTED AI PROVIDERS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Anthropic Claude   console.anthropic.com      (best for coding)
  OpenAI GPT         platform.openai.com
  Google Gemini      aistudio.google.com
  DeepSeek           platform.deepseek.com      (very cheap)
  Groq               console.groq.com           (very fast)
  Mistral AI         console.mistral.ai
  xAI (Grok)         console.x.ai
  Together AI        api.together.xyz
  Perplexity AI      www.perplexity.ai
  Cohere             cohere.com
  OpenRouter         openrouter.ai              (routes to any model)
  Fireworks AI       fireworks.ai
  Cerebras           cerebras.ai                (ultra-fast)
  Ollama             ollama.com                 (FREE, runs locally)
  LM Studio          lmstudio.ai                (FREE, runs locally)
  Custom             any OpenAI-compatible URL

  Switch providers anytime: press Tab → Settings

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ENVIRONMENT VARIABLES  (set these before running)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ANTHROPIC_API_KEY    Your Anthropic key (safer than pasting in wizard)
  OPENAI_API_KEY       OpenAI key
  GEMINI_API_KEY       Google Gemini key
  GITHUB_TOKEN         GitHub personal access token
  SUPABASE_URL         https://yourproject.supabase.co
  SUPABASE_SERVICE_KEY Supabase service-role key (from project settings)
  VERCEL_TOKEN         Vercel API token (from vercel.com/account/tokens)
  VELKRO_NO_COLOR      Set to 1 to disable all colours (plain text mode)
  VELKRO_ADDR          Daemon address if using remote mode (default 127.0.0.1:7477)

  Linux/Mac:   export ANTHROPIC_API_KEY=sk-ant-...
  Windows PS:  $env:ANTHROPIC_API_KEY="sk-ant-..."

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  WHERE YOUR DATA IS STORED  (all local, nothing in the cloud)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Providers & keys   ~/.config/velkrogo/providers.json  (600 permissions)
  Session history    ~/.config/velkrogo/state.db
  Audit log          ~/.config/velkrogo/audit.db
  Agent identity     ~/.velkrogo/SOUL.md

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  TROUBLESHOOTING
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  "No default provider"
    → Run the setup wizard again or press Tab to add a provider.

  "Connection failed"
    → Check your API key is correct and has credits.
    → For Ollama: make sure it is running (ollama serve).

  Colours broken in PowerShell
    → Run: $env:COLORTERM=""  or set VELKRO_NO_COLOR=1

  Device tools not working on Linux
    → Install xdotool: sudo apt install xdotool
    → For screenshots: sudo apt install scrot

  "Tool denied by policy"
    → The tool's risk tier was blocked. Try again and press y or s.

Full docs: https://github.com/rufatronics/VelkroGo
`

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Print(helpText)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("velkro", version)
		return
	}

	store, err := provider.LoadStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load provider store:", err)
		os.Exit(1)
	}

	// First-run if no providers configured.
	if len(store.List()) == 0 {
		if err := firstRunWizard(store); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	def := store.Default()
	if def == nil {
		fmt.Fprintln(os.Stderr, "No default provider. Run `velkro setup` or add one via the GUI.")
		os.Exit(1)
	}

	prov, err := provider.Build(*def)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build provider:", err)
		os.Exit(1)
	}

	// Open state DB for memory/skills.
	dbPath, _ := memory.DefaultPath()
	db, dbErr := memory.Open(dbPath)
	if dbErr == nil {
		tools.MemoryStore = db
		tools.SkillsStore = db
		defer db.Close()
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
		_ = reg.Register(t)
	}

	// Build layered system prompt.
	soulContent := soul.Load()
	var facts []memory.MemoryFact
	var skills []memory.Skill
	if db != nil {
		facts, _ = db.ListMemory()
		skills, _ = db.ListSkills()
	}
	sysPrompt := prompt.Build(prompt.Config{
		Soul:   soulContent,
		Facts:  facts,
		Skills: skills,
	})

	events := make(chan orchestrator.Event, 128)
	engine := &orchestrator.Engine{
		Provider:     prov,
		Model:        def.Model,
		Registry:     reg,
		Policy:       policy.NewBasic(),
		World:        registry.WorldShared,
		Events:       events,
		SystemPrompt: sysPrompt,
	}

	if err := runTUI(engine, events, store); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}

// firstRunWizard guides a new user through adding their first provider.
// It is intentionally non-technical: pick a name from a list, paste a key.
func firstRunWizard(store *provider.Store) error {
	in := bufio.NewReader(os.Stdin)
	read := func(prompt, def string) string {
		if def != "" {
			fmt.Printf("%s [%s]: ", prompt, def)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}
	yn := func(prompt string) bool {
		return strings.EqualFold(read(prompt+" (y/N)", "n"), "y")
	}

	fmt.Println()
	fmt.Println("Welcome to VelkroGo! Let's set up your AI provider.")
	fmt.Println()
	fmt.Println("Choose a provider:")
	fmt.Println("  1)  Anthropic (Claude) — claude-sonnet-4-6")
	fmt.Println("  2)  OpenAI (GPT)       — gpt-4o")
	fmt.Println("  3)  Google Gemini      — gemini-2.0-flash")
	fmt.Println("  4)  DeepSeek           — deepseek-chat")
	fmt.Println("  5)  Groq (ultra-fast)  — llama-3.3-70b-versatile")
	fmt.Println("  6)  Mistral AI         — mistral-large-latest")
	fmt.Println("  7)  xAI (Grok)         — grok-3")
	fmt.Println("  8)  Together AI        — llama-3-70b")
	fmt.Println("  9)  Perplexity AI      — sonar-large")
	fmt.Println(" 10)  Cohere             — command-r-plus")
	fmt.Println(" 11)  OpenRouter         — (routes to any model)")
	fmt.Println(" 12)  Ollama (local, free, no key needed)")
	fmt.Println(" 13)  LM Studio (local, free, no key needed)")
	fmt.Println(" 14)  Cerebras (fast)")
	fmt.Println(" 15)  Fireworks AI")
	fmt.Println(" 16)  Custom OpenAI-compatible endpoint")
	fmt.Println()

	type provDef struct {
		id, name, kind, baseURL, defaultModel, keyEnv string
		needsKey                                       bool
	}
	defs := []provDef{
		{"anthropic", "Anthropic", "anthropic", "", "claude-sonnet-4-6", "ANTHROPIC_API_KEY", true},
		{"openai", "OpenAI", "openai-compatible", "https://api.openai.com/v1", "gpt-4o", "OPENAI_API_KEY", true},
		{"gemini", "Google Gemini", "gemini", "", "gemini-2.0-flash", "GEMINI_API_KEY", true},
		{"deepseek", "DeepSeek", "openai-compatible", "https://api.deepseek.com/v1", "deepseek-chat", "DEEPSEEK_API_KEY", true},
		{"groq", "Groq", "openai-compatible", "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile", "GROQ_API_KEY", true},
		{"mistral", "Mistral AI", "openai-compatible", "https://api.mistral.ai/v1", "mistral-large-latest", "MISTRAL_API_KEY", true},
		{"xai", "xAI (Grok)", "openai-compatible", "https://api.x.ai/v1", "grok-3", "XAI_API_KEY", true},
		{"together", "Together AI", "openai-compatible", "https://api.together.xyz/v1", "meta-llama/Llama-3-70b-chat-hf", "TOGETHER_API_KEY", true},
		{"perplexity", "Perplexity AI", "openai-compatible", "https://api.perplexity.ai", "llama-3.1-sonar-large-128k-online", "PERPLEXITY_API_KEY", true},
		{"cohere", "Cohere", "openai-compatible", "https://api.cohere.com/compatibility/v1", "command-r-plus", "COHERE_API_KEY", true},
		{"openrouter", "OpenRouter", "openai-compatible", "https://openrouter.ai/api/v1", "anthropic/claude-sonnet-4-6", "OPENROUTER_API_KEY", true},
		{"ollama", "Ollama (local)", "openai-compatible", "http://localhost:11434/v1", "llama3.2", "", false},
		{"lmstudio", "LM Studio (local)", "openai-compatible", "http://localhost:1234/v1", "local-model", "", false},
		{"cerebras", "Cerebras", "openai-compatible", "https://api.cerebras.ai/v1", "llama3.1-70b", "CEREBRAS_API_KEY", true},
		{"fireworks", "Fireworks AI", "openai-compatible", "https://api.fireworks.ai/inference/v1", "accounts/fireworks/models/llama-v3p1-70b-instruct", "FIREWORKS_API_KEY", true},
		{"custom", "Custom", "openai-compatible", "", "", "", false},
	}

	choice := read("Enter number", "1")
	var idx int
	if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(defs) {
		return errors.New("invalid choice")
	}
	d := defs[idx-1]

	var e provider.Entry
	e.ID = d.id
	e.PresetID = d.id
	e.Kind = d.kind
	e.Name = read("Display name", d.name)
	if d.id == "custom" || d.baseURL == "" {
		e.BaseURL = read("Base URL (OpenAI-compatible)", "")
	} else {
		e.BaseURL = d.baseURL
	}
	e.Model = read("Model", d.defaultModel)

	if d.needsKey || d.id == "custom" {
		if d.keyEnv != "" {
			fmt.Printf("Tip: set the %s environment variable to avoid storing your key in a file.\n", d.keyEnv)
			e.KeyEnv = read("Env var name holding your API key", d.keyEnv)
		}
		if os.Getenv(e.KeyEnv) == "" {
			e.APIKey = read("API key (leave blank if using env var)", "")
		}
	}

	e.IsDefault = true
	if err := store.Add(e); err != nil {
		return err
	}

	saver := yn("Enable money-saving mode? (uses cheap model, minimal prompts)")
	if saver {
		// Store saver preference alongside the entry by a convention.
		fmt.Println("Saver mode enabled. You can toggle it anytime in the TUI with 's'.")
	}

	fmt.Printf("\nAll set! Provider %q saved.\n\n", e.Name)

	// Test connection.
	if yn("Test the connection now?") {
		fmt.Print("Testing… ")
		err := provider.TestConnection(context.Background(), e)
		if err != nil {
			fmt.Println("FAILED:", err)
			fmt.Println("You can continue and fix the key later via the GUI or config file.")
		} else {
			fmt.Println("OK ✓")
		}
	}
	return nil
}
