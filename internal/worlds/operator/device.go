// Package operator provides World 2 (Operator) tools for device control.
package operator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// Screenshot captures the current screen and returns a base64-encoded PNG.
type Screenshot struct{}

func (Screenshot) Name() string         { return "screenshot" }
func (Screenshot) Description() string  { return "Take a screenshot of the current screen. Returns base64-encoded PNG." }
func (Screenshot) Tier() registry.Tier  { return registry.TierDeviceControl }
func (Screenshot) World() registry.World { return registry.WorldOperator }
func (Screenshot) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"save_path":{"type":"string","description":"Optional path to save the screenshot file"}}}`)
}
func (Screenshot) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		SavePath string `json:"save_path"`
	}
	_ = json.Unmarshal(args, &in)

	outPath := in.SavePath
	if outPath == "" {
		outPath = fmt.Sprintf("/tmp/velkro_screenshot_%d.png", time.Now().UnixMilli())
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("scrot"); err == nil {
			cmd = exec.CommandContext(ctx, "scrot", outPath)
		} else if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			cmd = exec.CommandContext(ctx, "gnome-screenshot", "-f", outPath)
		} else {
			cmd = exec.CommandContext(ctx, "import", "-window", "root", outPath)
		}
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms,System.Drawing; $b=New-Object System.Drawing.Bitmap([System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); $g=[System.Drawing.Graphics]::FromImage($b); $g.CopyFromScreen(0,0,0,0,$b.Size); $b.Save('%s')`, outPath))
	default:
		return registry.Result{IsError: true, Content: "screenshot not supported on " + runtime.GOOS}, nil
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return registry.Result{IsError: true, Content: string(out) + ": " + err.Error()}, nil
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return registry.Result{Content: "screenshot saved to " + outPath}, nil
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	if in.SavePath == "" {
		_ = os.Remove(outPath)
	}
	return registry.Result{Content: "data:image/png;base64," + b64}, nil
}

// MouseClick clicks at screen coordinates.
type MouseClick struct{}

func (MouseClick) Name() string         { return "mouse_click" }
func (MouseClick) Description() string  { return "Click the mouse at screen coordinates (x, y)." }
func (MouseClick) Tier() registry.Tier  { return registry.TierDeviceControl }
func (MouseClick) World() registry.World { return registry.WorldOperator }
func (MouseClick) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"},"y":{"type":"integer"},"button":{"type":"string","enum":["left","right","middle"]}},"required":["x","y"]}`)
}
func (MouseClick) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Button string `json:"button"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.Button == "" {
		in.Button = "left"
	}
	switch runtime.GOOS {
	case "linux":
		btn := "1"
		switch in.Button {
		case "right":
			btn = "3"
		case "middle":
			btn = "2"
		}
		cmd := exec.CommandContext(ctx, "xdotool", "mousemove", "--sync",
			strconv.Itoa(in.X), strconv.Itoa(in.Y), "click", btn)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	case "windows":
		downFlag, upFlag := "0x0002", "0x0004"
		if in.Button == "right" {
			downFlag, upFlag = "0x0008", "0x0010"
		}
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object System.Drawing.Point(%d,%d); $sig='[DllImport("user32.dll")] public static extern void mouse_event(uint f,int x,int y,int d,int e);'; $t=Add-Type -MemberDefinition $sig -Name U -Namespace W -PassThru; $t::mouse_event(%s,0,0,0,0); $t::mouse_event(%s,0,0,0,0)`, in.X, in.Y, downFlag, upFlag)
		cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	default:
		return registry.Result{IsError: true, Content: "mouse_click not supported on " + runtime.GOOS}, nil
	}
	return registry.Result{Content: fmt.Sprintf("clicked %s at (%d, %d)", in.Button, in.X, in.Y)}, nil
}

// MouseMove moves the mouse cursor.
type MouseMove struct{}

func (MouseMove) Name() string         { return "mouse_move" }
func (MouseMove) Description() string  { return "Move the mouse cursor to screen coordinates (x, y)." }
func (MouseMove) Tier() registry.Tier  { return registry.TierDeviceControl }
func (MouseMove) World() registry.World { return registry.WorldOperator }
func (MouseMove) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"},"y":{"type":"integer"}},"required":["x","y"]}`)
}
func (MouseMove) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	switch runtime.GOOS {
	case "linux":
		cmd := exec.CommandContext(ctx, "xdotool", "mousemove", "--sync", strconv.Itoa(in.X), strconv.Itoa(in.Y))
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	case "windows":
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object System.Drawing.Point(%d,%d)`, in.X, in.Y)
		cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	default:
		return registry.Result{IsError: true, Content: "mouse_move not supported on " + runtime.GOOS}, nil
	}
	return registry.Result{Content: fmt.Sprintf("mouse moved to (%d, %d)", in.X, in.Y)}, nil
}

// KeyboardType types text using the keyboard.
type KeyboardType struct{}

func (KeyboardType) Name() string         { return "keyboard_type" }
func (KeyboardType) Description() string  { return "Type text using the keyboard (simulates key presses)." }
func (KeyboardType) Tier() registry.Tier  { return registry.TierDeviceControl }
func (KeyboardType) World() registry.World { return registry.WorldOperator }
func (KeyboardType) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"delay_ms":{"type":"integer","description":"Delay between keystrokes in ms (default 50)"}},"required":["text"]}`)
}
func (KeyboardType) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Text    string `json:"text"`
		DelayMS int    `json:"delay_ms"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.DelayMS == 0 {
		in.DelayMS = 50
	}
	switch runtime.GOOS {
	case "linux":
		cmd := exec.CommandContext(ctx, "xdotool", "type", "--delay", strconv.Itoa(in.DelayMS), "--", in.Text)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	case "windows":
		safe := strings.ReplaceAll(in.Text, "'", "''")
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`, safe)
		cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	default:
		return registry.Result{IsError: true, Content: "keyboard_type not supported on " + runtime.GOOS}, nil
	}
	return registry.Result{Content: fmt.Sprintf("typed %d characters", len(in.Text))}, nil
}

// KeyPress sends a key or key combination.
type KeyPress struct{}

func (KeyPress) Name() string         { return "key_press" }
func (KeyPress) Description() string  { return "Press a key or key combination (e.g. 'ctrl+c', 'Return', 'alt+F4')." }
func (KeyPress) Tier() registry.Tier  { return registry.TierDeviceControl }
func (KeyPress) World() registry.World { return registry.WorldOperator }
func (KeyPress) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"key":{"type":"string","description":"Key or combo, e.g. 'ctrl+c', 'Return', 'alt+Tab'"}},"required":["key"]}`)
}
func (KeyPress) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	switch runtime.GOOS {
	case "linux":
		cmd := exec.CommandContext(ctx, "xdotool", "key", "--", in.Key)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	case "windows":
		mapped := mapWinKey(in.Key)
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`, mapped)
		cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return registry.Result{IsError: true, Content: string(out)}, nil
		}
	default:
		return registry.Result{IsError: true, Content: "key_press not supported on " + runtime.GOOS}, nil
	}
	return registry.Result{Content: "pressed: " + in.Key}, nil
}

func mapWinKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "ctrl+", "^")
	key = strings.ReplaceAll(key, "alt+", "%")
	key = strings.ReplaceAll(key, "shift+", "+")
	key = strings.ReplaceAll(key, "return", "{ENTER}")
	key = strings.ReplaceAll(key, "escape", "{ESC}")
	key = strings.ReplaceAll(key, "tab", "{TAB}")
	return key
}

// OpenApp launches an application by name.
type OpenApp struct{}

func (OpenApp) Name() string         { return "open_app" }
func (OpenApp) Description() string  { return "Open an application by name or path (e.g. 'firefox', 'notepad')." }
func (OpenApp) Tier() registry.Tier  { return registry.TierDeviceControl }
func (OpenApp) World() registry.World { return registry.WorldOperator }
func (OpenApp) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"app":{"type":"string","description":"Application name or path"},"args":{"type":"array","items":{"type":"string"}}},"required":["app"]}`)
}
func (OpenApp) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		App  string   `json:"app"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.CommandContext(ctx, in.App, in.Args...)
	case "windows":
		psArgs := []string{"-Command", "Start-Process", "-FilePath", fmt.Sprintf("'%s'", in.App)}
		if len(in.Args) > 0 {
			psArgs = append(psArgs, "-ArgumentList", fmt.Sprintf("'%s'", strings.Join(in.Args, "','")))
		}
		cmd = exec.CommandContext(ctx, "powershell", psArgs...)
	default:
		return registry.Result{IsError: true, Content: "open_app not supported on " + runtime.GOOS}, nil
	}
	if err := cmd.Start(); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "launched: " + in.App}, nil
}

// AllOperatorTools returns all device control tools.
func AllOperatorTools() []registry.Tool {
	return []registry.Tool{
		Screenshot{},
		MouseClick{},
		MouseMove{},
		KeyboardType{},
		KeyPress{},
		OpenApp{},
	}
}
