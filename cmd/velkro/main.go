// Command velkro is the VelkroGo client. Phase 1 runs the agent engine
// in-process behind a Bubble Tea TUI; later phases move the engine into the
// velkrod daemon and this becomes a thin API client. See ARCHITECTURE.md §6.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rufatronics/velkrogo/internal/config"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/provider/anthropic"
	"github.com/rufatronics/velkrogo/internal/provider/openaicompat"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/tools"
)

func main() {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotConfigured) {
		cfg, err = firstRunWizard()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "velkro:", err)
		os.Exit(1)
	}

	prov, err := buildProvider(cfg.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, "velkro:", err)
		os.Exit(1)
	}

	reg := registry.NewMemory()
	for _, t := range []registry.Tool{tools.ReadFile{}, tools.ListDir{}, tools.WriteFile{}} {
		if err := reg.Register(t); err != nil {
			fmt.Fprintln(os.Stderr, "velkro:", err)
			os.Exit(1)
		}
	}

	mode := orchestrator.ModeNormal
	if cfg.SaverMode {
		mode = orchestrator.ModeSaver
	}

	events := make(chan orchestrator.Event, 64)
	engine := &orchestrator.Engine{
		Provider: prov,
		Model:    cfg.Provider.Model,
		Registry: reg,
		Policy:   policy.NewBasic(),
		Mode:     mode,
		World:    registry.WorldShared,
		Events:   events,
	}

	if err := runTUI(engine, events, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "velkro:", err)
		os.Exit(1)
	}
}

func buildProvider(pc config.ProviderConfig) (provider.Provider, error) {
	switch pc.Kind {
	case "anthropic":
		return anthropic.New(pc.Key(), pc.BaseURL), nil
	case "openai-compatible":
		if pc.BaseURL == "" {
			return nil, fmt.Errorf("provider %q needs a base_url", pc.Name)
		}
		return openaicompat.New(pc.Name, pc.Key(), pc.BaseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q", pc.Kind)
	}
}

// firstRunWizard is a plain-stdin setup flow (runs before the TUI starts).
// There is deliberately no default provider: the user picks.
func firstRunWizard() (config.Config, error) {
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

	fmt.Println("VelkroGo first-time setup")
	fmt.Println("  1) Anthropic (Claude)")
	fmt.Println("  2) OpenAI")
	fmt.Println("  3) Ollama (local)")
	fmt.Println("  4) Custom OpenAI-compatible endpoint")

	var pc config.ProviderConfig
	switch read("Choose a provider (1-4)", "") {
	case "1":
		pc = config.ProviderConfig{Kind: "anthropic", Name: "anthropic",
			Model:  read("Model", "claude-sonnet-4-6"),
			KeyEnv: read("Env var holding your API key", "ANTHROPIC_API_KEY")}
	case "2":
		pc = config.ProviderConfig{Kind: "openai-compatible", Name: "openai",
			BaseURL: "https://api.openai.com/v1",
			Model:   read("Model", "gpt-4o"),
			KeyEnv:  read("Env var holding your API key", "OPENAI_API_KEY")}
	case "3":
		pc = config.ProviderConfig{Kind: "openai-compatible", Name: "ollama",
			BaseURL: read("Base URL", "http://localhost:11434/v1"),
			Model:   read("Model", "llama3.1")}
	case "4":
		pc = config.ProviderConfig{Kind: "openai-compatible",
			Name:    read("Provider name", "custom"),
			BaseURL: read("Base URL (OpenAI-compatible, ends with /v1)", ""),
			Model:   read("Model", ""),
			KeyEnv:  read("Env var holding your API key (blank if none)", "")}
	default:
		return config.Config{}, fmt.Errorf("setup cancelled")
	}

	saver := strings.EqualFold(read("Enable money-saving mode? (y/N)", "n"), "y")
	cfg := config.Config{Provider: pc, SaverMode: saver}
	if err := config.Save(cfg); err != nil {
		return config.Config{}, err
	}
	path, _ := config.Path()
	fmt.Println("Saved to", path)
	return cfg, nil
}
