package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// RunShell is a T3 (device control) tool. Every invocation is gated by the
// policy engine and requires user approval unless a broad grant exists.
type RunShell struct{}

func (RunShell) Name() string          { return "run_shell" }
func (RunShell) Tier() registry.Tier   { return registry.TierDeviceControl }
func (RunShell) World() registry.World { return registry.WorldShared }
func (RunShell) Description() string {
	return "Run a shell command on the host and return stdout+stderr. Requires user approval."
}
func (RunShell) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The command to run"},"timeout_secs":{"type":"integer","description":"Timeout in seconds (default 30, max 300)"}},"required":["command"]}`)
}

func (RunShell) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Command     string `json:"command"`
		TimeoutSecs int    `json:"timeout_secs"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	to := in.TimeoutSecs
	if to <= 0 {
		to = 30
	}
	if to > 300 {
		to = 300
	}
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx2, "cmd", "/C", in.Command)
	} else {
		cmd = exec.CommandContext(ctx2, "sh", "-c", in.Command)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if len(text) > 8000 {
		text = text[:8000] + "\n…[output truncated]"
	}
	if err != nil {
		return registry.Result{Content: fmt.Sprintf("exit error: %v\n%s", err, text), IsError: true}, nil
	}
	if text == "" {
		text = "(no output)"
	}
	return registry.Result{Content: text}, nil
}
