package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/rufatronics/velkrogo/internal/integrations/supabase"
	"github.com/rufatronics/velkrogo/internal/integrations/vercel"
	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/prompt"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/reasoning"
	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/soul"
	"github.com/rufatronics/velkrogo/internal/tools"
	"github.com/rufatronics/velkrogo/internal/worlds/coder"
	"github.com/rufatronics/velkrogo/internal/worlds/operator"

	_ "github.com/rufatronics/velkrogo/internal/provider/anthropic"
	_ "github.com/rufatronics/velkrogo/internal/provider/gemini"
	_ "github.com/rufatronics/velkrogo/internal/provider/openaicompat"
)

var version = "dev"

// chatMsg is one line in the transcript.
type chatMsg struct {
	role    string // "you" | "agent" | "tool" | "error"
	content string
	ts      time.Time
}

// VelkroApp is the main application struct.
type VelkroApp struct {
	fyneApp fyne.App
	win     fyne.Window
	engine  *orchestrator.Engine
	store   *provider.Store

	mu       sync.Mutex
	messages []chatMsg
	planSteps []orchestrator.Step

	msgList   *widget.List
	planList  *widget.List
	input     *widget.Entry
	sendBtn   *widget.Button
	stopBtn   *widget.Button
	statusLbl *widget.Label
	tokenLbl  *widget.Label
	modeLbl   *widget.Label

	inToks  int
	outToks int
	busy    bool
	cancel  context.CancelFunc

	approvalCh chan approvalResult
	questionCh chan questionResult
}

type approvalResult struct {
	approved bool
	session  bool
}

type questionResult struct {
	answer string
}

func main() {
	a := app.NewWithID("com.rufatronics.velkrogo")
	a.Settings().SetTheme(theme.DarkTheme())

	store, err := provider.LoadStore()
	if err != nil {
		dialog.ShowError(err, nil)
		return
	}

	va := &VelkroApp{
		fyneApp: a,
		store:   store,
	}

	va.win = a.NewWindow("VelkroGo")
	va.win.Resize(fyne.NewSize(1100, 720))
	va.win.SetMaster()

	// First-run: no providers configured.
	if len(store.List()) == 0 {
		va.showSetupDialog(func() {
			va.init()
			va.win.ShowAndRun()
		})
		a.Run()
		return
	}

	va.init()
	va.win.ShowAndRun()
}

// init wires the engine and builds the UI.
func (va *VelkroApp) init() {
	def := va.store.Default()
	if def == nil {
		return
	}

	prov, err := provider.Build(*def)
	if err != nil {
		dialog.ShowError(err, va.win)
		return
	}

	// Open state DB for memory/skills.
	dbPath, _ := memory.DefaultPath()
	db, dbErr := memory.Open(dbPath)
	if dbErr == nil {
		tools.MemoryStore = db
		tools.SkillsStore = db
	}

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

	events := make(chan orchestrator.Event, 256)
	va.engine = &orchestrator.Engine{
		Provider:     prov,
		Model:        def.Model,
		Registry:     reg,
		Policy:       policy.NewBasic(),
		World:        registry.WorldShared,
		Events:       events,
		Approver:     va,
		Asker:        va,
		SystemPrompt: sysPrompt,
	}

	va.buildUI(def)
	go va.drainEvents(events)
}

// buildUI constructs the entire window layout.
func (va *VelkroApp) buildUI(def *provider.Entry) {
	// ── Chat list ──────────────────────────────────────────────────────────
	va.msgList = widget.NewList(
		func() int {
			va.mu.Lock()
			defer va.mu.Unlock()
			return len(va.messages)
		},
		func() fyne.CanvasObject {
			role := widget.NewLabel("")
			role.TextStyle = fyne.TextStyle{Bold: true}
			role.Resize(fyne.NewSize(60, 20))
			body := widget.NewLabel("")
			body.Wrapping = fyne.TextWrapWord
			return container.NewBorder(nil, nil, role, nil, body)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			va.mu.Lock()
			if id >= len(va.messages) {
				va.mu.Unlock()
				return
			}
			msg := va.messages[id]
			va.mu.Unlock()

			c := obj.(*fyne.Container)
			role := c.Objects[1].(*widget.Label)
			body := c.Objects[0].(*widget.Label)

			switch msg.role {
			case "you":
				role.SetText("You")
				role.Importance = widget.HighImportance
			case "agent":
				role.SetText("Agent")
				role.Importance = widget.SuccessImportance
			case "tool":
				role.SetText("Tool")
				role.Importance = widget.LowImportance
			case "error":
				role.SetText("Error")
				role.Importance = widget.DangerImportance
			}
			body.SetText(msg.content)
		},
	)

	// ── Plan list ──────────────────────────────────────────────────────────
	va.planList = widget.NewList(
		func() int {
			va.mu.Lock()
			defer va.mu.Unlock()
			return len(va.planSteps)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			va.mu.Lock()
			if id >= len(va.planSteps) {
				va.mu.Unlock()
				return
			}
			step := va.planSteps[id]
			va.mu.Unlock()
			icons := map[orchestrator.StepStatus]string{
				orchestrator.StepPending: "○",
				orchestrator.StepActive:  "▶",
				orchestrator.StepDone:    "✓",
				orchestrator.StepBlocked: "✗",
			}
			obj.(*widget.Label).SetText(fmt.Sprintf("%s %s", icons[step.Status], step.Title))
		},
	)

	planHeader := widget.NewLabelWithStyle("Plan", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	planPanel := container.NewBorder(planHeader, nil, nil, nil, va.planList)

	// ── Status bar labels ──────────────────────────────────────────────────
	va.statusLbl = widget.NewLabel("Ready")
	va.tokenLbl = widget.NewLabel("0 / 0 tok")
	va.modeLbl = widget.NewLabel("Normal mode")

	providerLbl := widget.NewLabelWithStyle(
		fmt.Sprintf("%s · %s", def.Name, def.Model),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
	)

	statusBar := container.NewHBox(
		canvas.NewCircle(theme.SuccessColor()),
		va.statusLbl,
		layout.NewSpacer(),
		providerLbl,
		widget.NewSeparator(),
		va.modeLbl,
		widget.NewSeparator(),
		va.tokenLbl,
	)

	// ── Input row ─────────────────────────────────────────────────────────
	va.input = widget.NewMultiLineEntry()
	va.input.SetPlaceHolder("Type a task and press Ctrl+Enter to send…")
	va.input.SetMinRowsVisible(2)
	va.input.OnSubmitted = func(_ string) {} // handled by key binding

	va.sendBtn = widget.NewButtonWithIcon("Send", theme.MailSendIcon(), va.send)
	va.sendBtn.Importance = widget.HighImportance

	va.stopBtn = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), va.stop)
	va.stopBtn.Importance = widget.DangerImportance
	va.stopBtn.Disable()

	modeBtn := widget.NewButton("💰 Saver mode", va.toggleMode)

	inputRow := container.NewBorder(
		nil, nil, nil,
		container.NewVBox(va.sendBtn, va.stopBtn, modeBtn),
		va.input,
	)

	// ── Toolbar ───────────────────────────────────────────────────────────
	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), va.showSettings)
	aboutBtn := widget.NewButtonWithIcon("About", theme.InfoIcon(), va.showAbout)
	toolbar := container.NewHBox(
		widget.NewLabelWithStyle("⚡ VelkroGo", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		settingsBtn, aboutBtn,
	)

	// ── Keyboard shortcut: Ctrl+Enter sends ───────────────────────────────
	va.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyReturn,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) { va.send() })

	// ── Root layout ───────────────────────────────────────────────────────
	chatPanel := container.NewBorder(nil, nil, nil, nil, va.msgList)
	split := container.NewHSplit(chatPanel, planPanel)
	split.SetOffset(0.72)

	root := container.NewBorder(toolbar, container.NewBorder(inputRow, statusBar, nil, nil), nil, nil, split)
	va.win.SetContent(root)

	// ── App menu ──────────────────────────────────────────────────────────
	va.win.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Settings", va.showSettings),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() { va.fyneApp.Quit() }),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", va.showAbout),
		),
	))
}

// ── Agent control ──────────────────────────────────────────────────────────

func (va *VelkroApp) send() {
	text := strings.TrimSpace(va.input.Text)
	if text == "" || va.busy {
		return
	}
	va.input.SetText("")
	va.appendMsg("you", text)
	va.setBusy(true)

	ctx, cancel := context.WithCancel(context.Background())
	va.mu.Lock()
	va.cancel = cancel
	va.mu.Unlock()

	go func() {
		defer va.setBusy(false)
		if err := va.engine.Run(ctx, text); err != nil && ctx.Err() == nil {
			va.appendMsg("error", err.Error())
		}
	}()
}

func (va *VelkroApp) stop() {
	va.mu.Lock()
	if va.cancel != nil {
		va.cancel()
	}
	va.mu.Unlock()
	va.setBusy(false)
}

func (va *VelkroApp) toggleMode() {
	if va.engine.Mode == orchestrator.ModeNormal {
		va.engine.Mode = orchestrator.ModeSaver
		va.modeLbl.SetText("💰 Saver mode")
	} else {
		va.engine.Mode = orchestrator.ModeNormal
		va.modeLbl.SetText("Normal mode")
	}
}

func (va *VelkroApp) setBusy(b bool) {
	va.mu.Lock()
	va.busy = b
	va.mu.Unlock()
	if b {
		va.sendBtn.Disable()
		va.stopBtn.Enable()
		va.statusLbl.SetText("Running…")
	} else {
		va.sendBtn.Enable()
		va.stopBtn.Disable()
		va.statusLbl.SetText("Ready")
	}
}

// ── Message helpers ────────────────────────────────────────────────────────

func (va *VelkroApp) appendMsg(role, content string) {
	va.mu.Lock()
	va.messages = append(va.messages, chatMsg{role: role, content: content, ts: time.Now()})
	n := len(va.messages)
	va.mu.Unlock()
	va.msgList.Refresh()
	va.msgList.ScrollTo(n - 1)
}

// ── Event drain ────────────────────────────────────────────────────────────

func (va *VelkroApp) drainEvents(ch <-chan orchestrator.Event) {
	for ev := range ch {
		switch ev.Kind {
		case "text":
			va.appendMsg("agent", ev.Text)
		case "tool_start":
			va.appendMsg("tool", "→ "+ev.Tool+"  "+ev.Text)
		case "tool_done":
			va.appendMsg("tool", "← "+ev.Tool+": "+firstLine(ev.Text))
		case "plan":
			if ev.Plan != nil {
				va.mu.Lock()
				va.planSteps = ev.Plan.Steps
				va.mu.Unlock()
				va.planList.Refresh()
			}
		case "usage":
			va.mu.Lock()
			va.inToks += ev.InToks
			va.outToks += ev.OutToks
			in, out := va.inToks, va.outToks
			va.mu.Unlock()
			va.tokenLbl.SetText(fmt.Sprintf("%d / %d tok", in, out))
		case "error":
			va.appendMsg("error", ev.Text)
		}
	}
}

// ── Approver (implements orchestrator.Approver) ────────────────────────────

func (va *VelkroApp) Approve(ctx context.Context, tool registry.Tool, preview string) (bool, *policy.Grant, error) {
	result := make(chan approvalResult, 1)

	va.fyneApp.SendNotification(&fyne.Notification{
		Title:   "VelkroGo — Approval Required",
		Content: fmt.Sprintf("%s wants to run: %s", tool.Name(), firstLine(preview)),
	})

	// Build dialog on the UI goroutine via a closure sent to the event queue.
	go func() {
		previewLabel := widget.NewLabel(truncate(preview, 300))
		previewLabel.Wrapping = fyne.TextWrapWord

		tierInfo := widget.NewLabelWithStyle(
			fmt.Sprintf("Tool: %s   Risk tier: T%d", tool.Name(), tool.Tier()),
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
		)

		content := container.NewVBox(tierInfo, widget.NewSeparator(), previewLabel)

		d := dialog.NewCustom("Approval Required", "Deny", content, va.win)
		d.SetOnClosed(func() {
			select {
			case result <- approvalResult{approved: false}:
			default:
			}
		})

		allowOnce := widget.NewButton("Allow once", func() {
			result <- approvalResult{approved: true, session: false}
			d.Hide()
		})
		allowOnce.Importance = widget.HighImportance

		allowSession := widget.NewButton("Allow for session", func() {
			result <- approvalResult{approved: true, session: true}
			d.Hide()
		})

		deny := widget.NewButton("Deny", func() {
			result <- approvalResult{approved: false}
			d.Hide()
		})
		deny.Importance = widget.DangerImportance

		btns := container.NewHBox(allowOnce, allowSession, deny)
		full := container.NewVBox(content, btns)
		d2 := dialog.NewCustomWithoutButtons("⚠ Approval Required", full, va.win)
		d2.Show()
	}()

	select {
	case r := <-result:
		if !r.approved {
			return false, nil, nil
		}
		var grant *policy.Grant
		if r.session {
			grant = &policy.Grant{Capability: tool.Name(), Scope: "*"}
		}
		return true, grant, nil
	case <-ctx.Done():
		return false, nil, ctx.Err()
	}
}

// ── Asker (implements reasoning.Asker) ────────────────────────────────────

func (va *VelkroApp) Ask(ctx context.Context, qs []reasoning.Question) ([]reasoning.Answer, error) {
	q := qs[0]
	result := make(chan string, 1)

	go func() {
		var selected string
		radios := widget.NewRadioGroup(func() []string {
			opts := make([]string, len(q.Options))
			for i, o := range q.Options {
				opts[i] = o.Label
			}
			return opts
		}(), func(v string) { selected = v })

		other := widget.NewEntry()
		other.SetPlaceHolder("Or type a custom answer…")

		content := container.NewVBox(
			widget.NewLabel(q.Prompt),
			widget.NewSeparator(),
			radios,
			widget.NewLabel(""),
			other,
		)

		dialog.ShowCustomConfirm("Question", "Submit", "Skip", content, func(ok bool) {
			if !ok {
				result <- ""
				return
			}
			ans := strings.TrimSpace(other.Text)
			if ans == "" {
				ans = selected
			}
			result <- ans
		}, va.win)
	}()

	select {
	case ans := <-result:
		return []reasoning.Answer{{Selected: []string{ans}}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ── Settings dialog ────────────────────────────────────────────────────────

func (va *VelkroApp) showSettings() {
	providers := va.store.List()

	list := widget.NewList(
		func() int { return len(providers) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			detail := widget.NewLabel("")
			detail.Importance = widget.LowImportance
			defBadge := widget.NewLabel("[default]")
			defBadge.Importance = widget.SuccessImportance
			return container.NewHBox(container.NewVBox(name, detail), layout.NewSpacer(), defBadge)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(providers) {
				return
			}
			p := providers[id]
			c := obj.(*fyne.Container)
			inner := c.Objects[0].(*fyne.Container)
			inner.Objects[0].(*widget.Label).SetText(p.Name)
			inner.Objects[1].(*widget.Label).SetText(p.Kind + " · " + p.Model)
			badge := c.Objects[2].(*widget.Label)
			if p.IsDefault {
				badge.SetText("[default]")
			} else {
				badge.SetText("")
			}
		},
	)

	var selected int = -1
	list.OnSelected = func(id widget.ListItemID) { selected = int(id) }

	setDefaultBtn := widget.NewButton("Set as Default", func() {
		if selected < 0 || selected >= len(providers) {
			return
		}
		_ = va.store.SetDefault(providers[selected].ID)
		providers = va.store.List()
		list.Refresh()
	})

	removeBtn := widget.NewButton("Remove", func() {
		if selected < 0 || selected >= len(providers) {
			return
		}
		_ = va.store.Remove(providers[selected].ID)
		providers = va.store.List()
		selected = -1
		list.Refresh()
	})
	removeBtn.Importance = widget.DangerImportance

	addBtn := widget.NewButtonWithIcon("Add Provider", theme.ContentAddIcon(), func() {
		va.showAddProviderDialog(func() {
			providers = va.store.List()
			list.Refresh()
		})
	})
	addBtn.Importance = widget.HighImportance

	btns := container.NewHBox(addBtn, layout.NewSpacer(), setDefaultBtn, removeBtn)
	content := container.NewBorder(nil, btns, nil, nil, list)

	d := dialog.NewCustom("Provider Settings", "Close", content, va.win)
	d.Resize(fyne.NewSize(560, 400))
	d.Show()
}

func (va *VelkroApp) showAddProviderDialog(onSave func()) {
	presets := []string{
		"Anthropic (Claude)", "OpenAI (GPT)", "Google Gemini",
		"DeepSeek", "Groq", "Mistral AI", "xAI (Grok)",
		"Together AI", "Perplexity AI", "Cohere", "OpenRouter",
		"Fireworks AI", "Cerebras", "Ollama (local)", "LM Studio (local)",
		"Custom",
	}

	presetKinds := map[string]provider.Entry{
		"Anthropic (Claude)": {Kind: "anthropic", Name: "Anthropic", Model: "claude-sonnet-4-6", KeyEnv: "ANTHROPIC_API_KEY"},
		"OpenAI (GPT)":       {Kind: "openai-compatible", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o", KeyEnv: "OPENAI_API_KEY"},
		"Google Gemini":      {Kind: "gemini", Name: "Google Gemini", Model: "gemini-2.0-flash", KeyEnv: "GEMINI_API_KEY"},
		"DeepSeek":           {Kind: "openai-compatible", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", KeyEnv: "DEEPSEEK_API_KEY"},
		"Groq":               {Kind: "openai-compatible", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", KeyEnv: "GROQ_API_KEY"},
		"Mistral AI":         {Kind: "openai-compatible", Name: "Mistral", BaseURL: "https://api.mistral.ai/v1", Model: "mistral-large-latest", KeyEnv: "MISTRAL_API_KEY"},
		"xAI (Grok)":         {Kind: "openai-compatible", Name: "xAI", BaseURL: "https://api.x.ai/v1", Model: "grok-3", KeyEnv: "XAI_API_KEY"},
		"Together AI":        {Kind: "openai-compatible", Name: "Together", BaseURL: "https://api.together.xyz/v1", Model: "meta-llama/Llama-3-70b-chat-hf", KeyEnv: "TOGETHER_API_KEY"},
		"Perplexity AI":      {Kind: "openai-compatible", Name: "Perplexity", BaseURL: "https://api.perplexity.ai", Model: "llama-3.1-sonar-large-128k-online", KeyEnv: "PERPLEXITY_API_KEY"},
		"Cohere":             {Kind: "openai-compatible", Name: "Cohere", BaseURL: "https://api.cohere.com/compatibility/v1", Model: "command-r-plus", KeyEnv: "COHERE_API_KEY"},
		"OpenRouter":         {Kind: "openai-compatible", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Model: "anthropic/claude-sonnet-4-6", KeyEnv: "OPENROUTER_API_KEY"},
		"Fireworks AI":       {Kind: "openai-compatible", Name: "Fireworks", BaseURL: "https://api.fireworks.ai/inference/v1", Model: "accounts/fireworks/models/llama-v3p1-70b-instruct", KeyEnv: "FIREWORKS_API_KEY"},
		"Cerebras":           {Kind: "openai-compatible", Name: "Cerebras", BaseURL: "https://api.cerebras.ai/v1", Model: "llama3.1-70b", KeyEnv: "CEREBRAS_API_KEY"},
		"Ollama (local)":     {Kind: "openai-compatible", Name: "Ollama", BaseURL: "http://localhost:11434/v1", Model: "llama3.2"},
		"LM Studio (local)":  {Kind: "openai-compatible", Name: "LM Studio", BaseURL: "http://localhost:1234/v1", Model: "local-model"},
		"Custom":             {Kind: "openai-compatible", Name: "Custom"},
	}

	presetSel := widget.NewSelect(presets, nil)
	presetSel.SetSelected(presets[0])

	nameEntry := widget.NewEntry()
	urlEntry := widget.NewEntry()
	modelEntry := widget.NewEntry()
	keyEntry := widget.NewPasswordEntry()
	keyEnvEntry := widget.NewEntry()
	hintLabel := widget.NewLabel("")
	hintLabel.Importance = widget.LowImportance

	fill := func(name string) {
		e := presetKinds[name]
		nameEntry.SetText(e.Name)
		urlEntry.SetText(e.BaseURL)
		modelEntry.SetText(e.Model)
		keyEnvEntry.SetText(e.KeyEnv)
		if e.KeyEnv != "" {
			hintLabel.SetText("Tip: set " + e.KeyEnv + " env var to avoid storing key in file")
		} else {
			hintLabel.SetText("No API key needed for local providers")
		}
	}
	presetSel.OnChanged = fill
	fill(presets[0])

	form := widget.NewForm(
		widget.NewFormItem("Preset", presetSel),
		widget.NewFormItem("Display name", nameEntry),
		widget.NewFormItem("Base URL", urlEntry),
		widget.NewFormItem("Model", modelEntry),
		widget.NewFormItem("API Key", keyEntry),
		widget.NewFormItem("Key env var", keyEnvEntry),
	)

	testResult := widget.NewLabel("")

	testBtn := widget.NewButton("Test Connection", func() {
		testResult.SetText("Testing…")
		preset := presetKinds[presetSel.Selected]
		e := provider.Entry{
			Kind:    preset.Kind,
			Name:    nameEntry.Text,
			BaseURL: urlEntry.Text,
			Model:   modelEntry.Text,
			APIKey:  keyEntry.Text,
			KeyEnv:  keyEnvEntry.Text,
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := provider.TestConnection(ctx, e); err != nil {
				testResult.SetText("✗ " + err.Error())
			} else {
				testResult.SetText("✓ Connection successful!")
			}
		}()
	})

	content := container.NewVBox(form, hintLabel, testBtn, testResult)

	dialog.ShowCustomConfirm("Add Provider", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		preset := presetKinds[presetSel.Selected]
		e := provider.Entry{
			ID:       fmt.Sprintf("%s-%d", presetSel.Selected, time.Now().Unix()),
			PresetID: presetSel.Selected,
			Kind:     preset.Kind,
			Name:     nameEntry.Text,
			BaseURL:  urlEntry.Text,
			Model:    modelEntry.Text,
			APIKey:   keyEntry.Text,
			KeyEnv:   keyEnvEntry.Text,
		}
		if len(va.store.List()) == 0 {
			e.IsDefault = true
		}
		_ = va.store.Add(e)
		onSave()
	}, va.win)
}

// ── First-run setup dialog ─────────────────────────────────────────────────

func (va *VelkroApp) showSetupDialog(onComplete func()) {
	welcome := widget.NewLabelWithStyle(
		"Welcome to VelkroGo!\nChoose an AI provider to get started.",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)

	va.win.SetContent(container.NewCenter(container.NewVBox(
		welcome,
		widget.NewButton("Set up provider →", func() {
			va.showAddProviderDialog(func() {
				onComplete()
			})
		}),
	)))
	va.win.Show()
}

// ── About dialog ───────────────────────────────────────────────────────────

func (va *VelkroApp) showAbout() {
	helpLabel := widget.NewLabel(guiHelpText)
	helpLabel.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(helpLabel)
	scroll.SetMinSize(fyne.NewSize(640, 480))
	dialog.ShowCustom("VelkroGo — Help & Feature Guide", "Close", scroll, va.win)
}

const guiHelpText = `VelkroGo Desktop App — Self-hosted AI agent for Windows & Linux

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
FIRST TIME? START HERE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

VelkroGo is an AI agent that runs entirely on your machine. It can write code, manage git repos, search the web, control your desktop, connect to Supabase and Vercel, and automate tasks — all with your approval before anything consequential happens. Nothing is sent anywhere except to the AI provider you choose.

Step 1: On first launch a setup dialog appears automatically. Pick your AI provider from the list and enter your API key.
Step 2: Type a task in the chat box and press Ctrl+Enter or click Send.
Step 3: The agent outlines a plan in the right panel, then executes it step by step.
Step 4: When it wants to do something risky it shows an approval popup — you decide.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
THE INTERFACE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Chat panel (left): Your full conversation with the agent. Every message, tool call, and result appears here.

Plan panel (right): The agent's numbered step-by-step plan for the current task. Watch it tick off steps in real time.

Chat box (bottom): Type your task here. Press Ctrl+Enter or click Send.

Stop button: Cancel the current task at any time. The agent stops after the current tool finishes.

Saver mode button (💰): Switches to a cheaper model with minimal prompts. Good for simple tasks to save API credits.

Provider/model display: Shows which AI provider and model is active. Click Settings to change it.

Token counter: Tracks how many tokens (words) you've used this session.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
KEYBOARD SHORTCUTS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Ctrl+Enter    Send your message
Escape        Cancel the current task

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
APPROVAL GATE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Before anything risky happens a popup appears:

  ⚠ Approval Required
  Tool: run_shell   Risk: T3
  sh -c "npm run build"
  [Allow once]  [Allow for session]  [Deny]

Allow once       — run this action, ask again next time
Allow for session — never ask for this tool again until you restart
Deny             — block the action; the agent notes it and adjusts

Risk tiers:
  T0  Read-only (read file, web search, git log)   — silent, auto-runs
  T1  Local write (write file, git commit)          — popup
  T2  External (git push, HTTP POST, PR)            — popup + preview
  T3  Device control (run shell, mouse, keyboard)   — popup
  T4  Self-modify (edit VelkroGo's own code)        — explicit accept

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
QUESTION BOX
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

When the agent is unsure it pauses and asks before guessing:

  Question: Which branch should I push to?
  ○ main
  ○ dev
  ○ Create a new branch
  [Or type a custom answer…]  [Submit]

This prevents wrong actions on ambiguous requests.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
FULL FEATURE LIST — WHAT THE AGENT CAN DO
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

FILE SYSTEM
  Read files, write files, list directories
  Create directories (make_dir)
  Delete files and folders (delete_path)
  Move and rename (move_path)
  Copy files (copy_file)

WEB
  Search the web — uses DuckDuckGo, no API key needed
  Download and read any web page (fetch_page)

SHELL
  Run any command in bash or PowerShell (run_shell)
  Always requires T3 approval. 30-second default timeout.

GIT — coding agent features
  git_status — see what changed in a repo
  git_diff — see the exact line-by-line changes
  git_log — view commit history
  git_clone — clone any GitHub or GitLab repo
  git_commit — stage everything and commit
  git_create_branch — create and switch to a new branch
  git_push — push to a remote (T2, always asks + shows preview)

GITHUB API  (requires GITHUB_TOKEN environment variable)
  github_list_prs — list open pull requests in a repo
  github_create_pr — open a new pull request
  github_create_issue — create a new issue with labels
  github_merge_pr — merge a pull request (squash/merge/rebase)

BUILD & TEST
  run_build — run the project's build command
  run_tests — run the test suite and report results

SUPABASE  (requires SUPABASE_URL + SUPABASE_SERVICE_KEY)
  supabase_select — query rows from any table with filters
  supabase_insert — insert a new row
  supabase_update — update existing rows by filter
  supabase_delete — delete rows by filter
  supabase_storage_upload — upload a local file to a storage bucket

VERCEL  (requires VERCEL_TOKEN)
  vercel_list_deployments — list recent deployments and their status
  vercel_deploy — trigger a new deployment
  vercel_set_env — set environment variables on a project

DEVICE CONTROL  (World 2 — Operator mode)
  screenshot — take a screenshot (returns image to agent)
  mouse_click — click at any (x, y) screen position
  mouse_move — move the cursor without clicking
  keyboard_type — type text character by character
  key_press — press key combos like ctrl+c, Return, alt+F4
  open_app — launch any application by name or path
  Linux: requires xdotool installed (sudo apt install xdotool)
  Linux: for screenshots: sudo apt install scrot

MEMORY  (persists across sessions and restarts)
  memory_set — store a fact: "Remember that my project is at ~/code"
  memory_get — recall a specific fact by its key
  memory_list — see everything the agent remembers
  memory_delete — forget a specific fact
  Facts are saved in ~/.config/velkrogo/state.db and recalled
  at the start of every conversation automatically.

SKILLS  (reusable named prompts)
  skills_save — save a reusable procedure by name
  skills_list — list all saved skills
  invoke_skill — run a skill by name
  skills_delete — delete a skill
  Example: "Save a skill called 'deploy' that runs vercel_deploy
  and then sends me a Slack message with the deployment URL."
  Then: "Run the deploy skill."

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SOUL.md — CUSTOMISE THE AGENT'S PERSONALITY
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Edit the file ~/.velkrogo/SOUL.md to change how the agent behaves. It is loaded at startup and becomes the first layer of every system prompt. The file is created automatically with sensible defaults.

Example instructions you can add:
  "Always respond in Spanish."
  "Prefer TypeScript over JavaScript in all new files."
  "Never delete files without asking, even for T1 operations."
  "This agent manages the production server at api.myapp.com."

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
LAYERED PROMPT ARCHITECTURE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Every request builds the AI's system prompt from these layers:

  Layer 0  Your SOUL.md identity file
  Layer 1  Session rules (restrictions for this session)
  Layer 2  Remembered facts (from memory_set)
  Layer 3  Available skills (from skills_save)
  Layer 4  Tool list (what tools are active)
  Layer 5  Mode hint (normal vs cost-saver)

This means the agent always knows your preferences and past facts without you repeating them.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SETTINGS — MANAGING PROVIDERS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Click Settings in the toolbar to:
  • See all configured providers
  • Add a new provider (16 presets + Custom)
  • Test connection — verify your key before saving
  • Set as Default — switch the active provider
  • Remove — delete a provider

You can have multiple providers and switch between them. Example: use Ollama (free, local) for quick tasks and Anthropic Claude for complex ones.

Supported providers:
  Anthropic Claude, OpenAI GPT, Google Gemini, DeepSeek, Groq,
  Mistral AI, xAI (Grok), Together AI, Perplexity AI, Cohere,
  OpenRouter, Fireworks AI, Cerebras, Ollama, LM Studio, Custom

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
ENVIRONMENT VARIABLES
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Set these before launching the app to avoid storing keys in files:

  ANTHROPIC_API_KEY    sk-ant-...
  OPENAI_API_KEY       sk-...
  GEMINI_API_KEY       AI...
  GITHUB_TOKEN         ghp_...
  SUPABASE_URL         https://yourproject.supabase.co
  SUPABASE_SERVICE_KEY eyJ...
  VERCEL_TOKEN         ...

Linux/Mac:  export ANTHROPIC_API_KEY=sk-ant-...
Windows PS: $env:ANTHROPIC_API_KEY="sk-ant-..."

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
WHERE YOUR DATA IS STORED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

All data stays on your machine. Nothing is uploaded to any cloud service except the prompts you send to your chosen AI provider.

  Provider keys    ~/.config/velkrogo/providers.json  (600 permissions)
  Sessions/memory  ~/.config/velkrogo/state.db
  Audit log        ~/.config/velkrogo/audit.db
  Agent identity   ~/.velkrogo/SOUL.md

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
TROUBLESHOOTING
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

"No provider configured"
  → Click Settings → Add Provider and enter your API key.

"Connection failed"
  → Check your API key is correct and has credits remaining.
  → For Ollama: run 'ollama serve' first.

App won't start on Linux
  → sudo apt-get install libgl1-mesa-dev xorg-dev

Device tools not working
  → sudo apt install xdotool scrot  (Linux)

"Tool denied by policy"
  → Try the task again and click Allow in the popup.

Full docs: https://github.com/rufatronics/VelkroGo`

// ── Helpers ────────────────────────────────────────────────────────────────

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
