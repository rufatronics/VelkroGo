package coder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// RunBuild runs a build command in the project directory (T1 — local).
type RunBuild struct{}

func (RunBuild) Name() string          { return "run_build" }
func (RunBuild) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (RunBuild) World() registry.World { return registry.WorldCoder }
func (RunBuild) Description() string   { return "Run a build command in a project directory (e.g. 'go build ./...', 'npm run build')." }
func (RunBuild) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Project directory"},"command":{"type":"string","description":"Build command"}},"required":["path","command"]}`)
}
func (RunBuild) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	return runCmd(ctx, args, 5*time.Minute)
}

// RunTests runs a test command in the project directory (T1 — local).
type RunTests struct{}

func (RunTests) Name() string          { return "run_tests" }
func (RunTests) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (RunTests) World() registry.World { return registry.WorldCoder }
func (RunTests) Description() string   { return "Run tests in a project directory and return the output." }
func (RunTests) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"command":{"type":"string","description":"Test command (e.g. 'go test ./...' or 'npm test')"}},"required":["path","command"]}`)
}
func (RunTests) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	return runCmd(ctx, args, 10*time.Minute)
}

func runCmd(ctx context.Context, args json.RawMessage, timeout time.Duration) (registry.Result, error) {
	var in struct {
		Path    string `json:"path"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx2, "sh", "-c", in.Command)
	cmd.Dir = in.Path
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if len(text) > 12000 {
		text = text[:12000] + "\n…[truncated]"
	}
	if err != nil {
		return registry.Result{Content: fmt.Sprintf("FAILED: %v\n%s", err, text), IsError: true}, nil
	}
	if text == "" {
		text = "(no output)"
	}
	return registry.Result{Content: text}, nil
}

func init() {
	// Append build/test tools to the list returned by CoderTools.
	// They're in the same package so we extend via a package-level slice instead.
}

// AllCoderTools returns all World 1 tools.
func AllCoderTools() []registry.Tool {
	return append(CoderTools(), RunBuild{}, RunTests{})
}
