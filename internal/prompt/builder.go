// Package prompt assembles the layered system prompt stack.
// Layer 0: SOUL.md identity
// Layer 1: Session rules
// Layer 2: Memory facts
// Layer 3: Available skills
// Layer 4: Mode hint
package prompt

import (
	"fmt"
	"strings"

	"github.com/rufatronics/velkrogo/internal/memory"
)

// Config holds everything needed to build the system prompt.
type Config struct {
	Soul      string
	Rules     []string
	Facts     []memory.MemoryFact
	Skills    []memory.Skill
	SaverMode bool
}

// Build assembles the full layered system prompt.
func Build(c Config) string {
	var b strings.Builder

	if c.Soul != "" {
		b.WriteString(c.Soul)
		b.WriteString("\n\n---\n\n")
	}

	if len(c.Rules) > 0 {
		b.WriteString("## Session Rules\n")
		for _, r := range c.Rules {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}

	if len(c.Facts) > 0 {
		b.WriteString("## Remembered Facts\n")
		for _, f := range c.Facts {
			fmt.Fprintf(&b, "- **%s**: %s\n", f.Key, f.Value)
		}
		b.WriteString("\n")
	}

	if len(c.Skills) > 0 {
		b.WriteString("## Available Skills\n")
		b.WriteString("You can invoke these reusable prompts by name using the invoke_skill tool:\n")
		for _, s := range c.Skills {
			fmt.Fprintf(&b, "- **%s**: %s\n", s.Name, s.Description)
		}
		b.WriteString("\n")
	}

	if c.SaverMode {
		b.WriteString("## Mode: Cost Saver\nBe brief. Use tools only when needed. Skip set_plan for simple tasks.\n")
	} else {
		b.WriteString("## Instructions\nBefore non-trivial tasks, call set_plan to outline numbered steps and update it as you progress. When a request is ambiguous, call ask_user with 2-4 options.\n")
	}

	return strings.TrimSpace(b.String())
}
