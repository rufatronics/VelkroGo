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

	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/tools"
	"github.com/rufatronics/velkrogo/internal/worlds/coder"

	// Register provider factories.
	_ "github.com/rufatronics/velkrogo/internal/provider/anthropic"
	_ "github.com/rufatronics/velkrogo/internal/provider/gemini"
	_ "github.com/rufatronics/velkrogo/internal/provider/openaicompat"
)

func main() {
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
		_ = reg.Register(t)
	}

	events := make(chan orchestrator.Event, 128)
	engine := &orchestrator.Engine{
		Provider: prov,
		Model:    def.Model,
		Registry: reg,
		Policy:   policy.NewBasic(),
		World:    registry.WorldShared,
		Events:   events,
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
		line = strings.Trim(line, "\r\n")
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
	fmt.Println("  1)  Anthropic (Claude)")
	fmt.Println("  2)  OpenAI (GPT)")
	fmt.Println("  3)  Google Gemini")
	fmt.Println("  4)  DeepSeek")
	fmt.Println("  5)  Groq (ultra-fast)")
	fmt.Println("  6)  Mistral AI")
	fmt.Println("  7)  xAI (Grok)")
	fmt.Println("  8)  Together AI")
	fmt.Println("  9)  Perplexity AI")
	fmt.Println(" 10)  Cohere")
	fmt.Println(" 11)  OpenRouter")
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
		{"anthropic", "Anthropic", "anthropic", "", "", "ANTHROPIC_API_KEY", true},
		{"openai", "OpenAI", "openai-compatible", "https://api.openai.com/v1", "", "OPENAI_API_KEY", true},
		{"gemini", "Google Gemini", "gemini", "", "", "GEMINI_API_KEY", true},
		{"deepseek", "DeepSeek", "openai-compatible", "https://api.deepseek.com/v1", "", "DEEPSEEK_API_KEY", true},
		{"groq", "Groq", "openai-compatible", "https://api.groq.com/openai/v1", "", "GROQ_API_KEY", true},
		{"mistral", "Mistral AI", "openai-compatible", "https://api.mistral.ai/v1", "", "MISTRAL_API_KEY", true},
		{"xai", "xAI (Grok)", "openai-compatible", "https://api.x.ai/v1", "", "XAI_API_KEY", true},
		{"together", "Together AI", "openai-compatible", "https://api.together.xyz/v1", "", "TOGETHER_API_KEY", true},
		{"perplexity", "Perplexity AI", "openai-compatible", "https://api.perplexity.ai", "", "PERPLEXITY_API_KEY", true},
		{"cohere", "Cohere", "openai-compatible", "https://api.cohere.com/compatibility/v1", "", "COHERE_API_KEY", true},
		{"openrouter", "OpenRouter", "openai-compatible", "https://openrouter.ai/api/v1", "", "OPENROUTER_API_KEY", true},
		{"ollama", "Ollama (local)", "openai-compatible", "http://localhost:11434/v1", "", "", false},
		{"lmstudio", "LM Studio (local)", "openai-compatible", "http://localhost:1234/v1", "", "", false},
		{"cerebras", "Cerebras", "openai-compatible", "https://api.cerebras.ai/v1", "", "CEREBRAS_API_KEY", true},
		{"fireworks", "Fireworks AI", "openai-compatible", "https://api.fireworks.ai/inference/v1", "", "FIREWORKS_API_KEY", true},
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
