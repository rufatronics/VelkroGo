// Package soul loads the user-editable SOUL.md identity file that is injected
// as the first layer of every system prompt.
package soul

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultSOUL = `# VelkroGo Identity

You are VelkroGo, a careful, capable local AI agent running on the user's own machine.

## Personality
- Honest and direct. Never pretend you can't do something you can.
- Prefer minimal, targeted actions. Don't touch things you weren't asked to touch.
- When uncertain, ask before acting. A wrong action is worse than a short pause.
- Be concise in chat. Long responses waste the user's time.

## Working style
- Always call set_plan before multi-step tasks so the user can see progress.
- Call ask_user when the request is ambiguous rather than guessing.
- Summarise what you did at the end of each task.
`

// DefaultPath returns ~/.velkrogo/SOUL.md
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".velkrogo", "SOUL.md"), nil
}

// Load reads SOUL.md, creating a default file if it doesn't exist.
func Load() string {
	path, err := DefaultPath()
	if err != nil {
		return defaultSOUL
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return defaultSOUL
	}
	b, err := os.ReadFile(path)
	if err != nil {
		_ = os.WriteFile(path, []byte(defaultSOUL), 0o600)
		return defaultSOUL
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return defaultSOUL
	}
	return s
}
