package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/reasoning"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// ---- bridge (approver + asker) ----

type approvalReq struct {
	tool    registry.Tool
	preview string
	reply   chan approvalResp
}

type approvalResp struct {
	approved bool
	grant    *policy.Grant
}

type questionReq struct {
	qs    []reasoning.Question
	reply chan []reasoning.Answer
}

type uiBridge struct {
	prog *tea.Program
}

func (b *uiBridge) Approve(ctx context.Context, tool registry.Tool, preview string) (bool, *policy.Grant, error) {
	req := approvalReq{tool: tool, preview: preview, reply: make(chan approvalResp, 1)}
	b.prog.Send(req)
	select {
	case r := <-req.reply:
		return r.approved, r.grant, nil
	case <-ctx.Done():
		return false, nil, ctx.Err()
	}
}

func (b *uiBridge) Ask(ctx context.Context, qs []reasoning.Question) ([]reasoning.Answer, error) {
	req := questionReq{qs: qs, reply: make(chan []reasoning.Answer, 1)}
	b.prog.Send(req)
	select {
	case a := <-req.reply:
		return a, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type turnDone struct{ err error }

// ---- TUI model ----

type tuiView int

const (
	viewChat tuiView = iota
	viewSettings
)

type uiState int

const (
	stateInput uiState = iota
	stateBusy
	stateApproval
	stateQuestion
)

type model struct {
	engine   *orchestrator.Engine
	store    *provider.Store
	state    uiState
	view     tuiView
	input    string
	lines    []string
	plan     *orchestrator.Plan
	approval *approvalReq
	question *questionReq
	inToks   int
	outToks  int
	width    int
	// Settings cursor
	settingsCursor int
}

var (
	styleUser   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleAgent  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleTool   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stylePlan   = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	styleHead   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	styleSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
)

func runTUI(engine *orchestrator.Engine, events chan orchestrator.Event, store *provider.Store) error {
	m := &model{
		engine: engine,
		store:  store,
		lines: []string{
			styleHead.Render("VelkroGo") + "  " + modeLine(engine) + "  " + styleDim.Render("Tab=settings  Ctrl+C=quit"),
		},
	}
	p := tea.NewProgram(m)

	bridge := &uiBridge{prog: p}
	engine.Approver = bridge
	engine.Asker = bridge

	go func() {
		for ev := range events {
			p.Send(ev)
		}
	}()

	_, err := p.Run()
	return err
}

func modeLine(e *orchestrator.Engine) string {
	mode := "normal"
	if e.Mode == orchestrator.ModeSaver {
		mode = "saver"
	}
	return styleDim.Render(fmt.Sprintf("[%s mode]", mode))
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case orchestrator.Event:
		return m.handleEvent(msg)

	case approvalReq:
		req := msg
		m.approval = &req
		m.state = stateApproval
		return m, nil

	case questionReq:
		req := msg
		m.question = &req
		m.state = stateQuestion
		return m, nil

	case turnDone:
		m.state = stateInput
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleEvent(ev orchestrator.Event) (tea.Model, tea.Cmd) {
	switch ev.Kind {
	case "text":
		m.lines = append(m.lines, styleAgent.Render(ev.Text))
	case "tool_start":
		m.lines = append(m.lines, styleTool.Render("→ "+ev.Tool+" "+ev.Text))
	case "tool_done":
		m.lines = append(m.lines, styleTool.Render("← "+ev.Tool+": "+firstLine(ev.Text)))
	case "plan":
		m.plan = ev.Plan
	case "usage":
		m.inToks += ev.InToks
		m.outToks += ev.OutToks
	case "error":
		m.lines = append(m.lines, styleErr.Render("error: "+ev.Text))
		m.state = stateInput
	}
	return m, nil
}

func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	// Tab switches views.
	if key.Type == tea.KeyTab && m.state == stateInput {
		if m.view == viewChat {
			m.view = viewSettings
		} else {
			m.view = viewChat
		}
		return m, nil
	}

	switch m.state {
	case stateApproval:
		switch key.String() {
		case "y":
			m.approval.reply <- approvalResp{approved: true}
			m.approval = nil; m.state = stateBusy
		case "s":
			m.approval.reply <- approvalResp{approved: true, grant: &policy.Grant{Capability: m.approval.tool.Name(), Scope: "*"}}
			m.approval = nil; m.state = stateBusy
		case "n", "esc":
			m.approval.reply <- approvalResp{approved: false}
			m.approval = nil; m.state = stateBusy
		}
		return m, nil

	case stateQuestion:
		q := m.question.qs[0]
		if n, err := strconv.Atoi(key.String()); err == nil && n >= 1 && n <= len(q.Options) {
			m.question.reply <- []reasoning.Answer{{Selected: []string{q.Options[n-1].Label}}}
			m.lines = append(m.lines, styleWarn.Render("you chose: "+q.Options[n-1].Label))
			m.question = nil; m.state = stateBusy
		}
		return m, nil

	case stateInput:
		if m.view == viewSettings {
			return m.handleSettingsKey(key)
		}
		return m.handleChatKey(key)
	}
	return m, nil
}

func (m *model) handleChatKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input)
		if text == "" {
			return m, nil
		}
		// 's' toggle shortcut.
		if text == "/saver" {
			if m.engine.Mode == orchestrator.ModeNormal {
				m.engine.Mode = orchestrator.ModeSaver
				m.lines = append(m.lines, styleDim.Render("Switched to saver mode"))
			} else {
				m.engine.Mode = orchestrator.ModeNormal
				m.lines = append(m.lines, styleDim.Render("Switched to normal mode"))
			}
			m.input = ""
			return m, nil
		}
		m.input = ""
		m.lines = append(m.lines, styleUser.Render("you: ")+text)
		m.state = stateBusy
		engine := m.engine
		return m, func() tea.Msg {
			return turnDone{err: engine.Run(context.Background(), text)}
		}
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	case tea.KeyRunes:
		m.input += string(key.Runes)
	case tea.KeySpace:
		m.input += " "
	}
	return m, nil
}

func (m *model) handleSettingsKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	providers := m.store.List()
	switch key.Type {
	case tea.KeyUp:
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case tea.KeyDown:
		if m.settingsCursor < len(providers)-1 {
			m.settingsCursor++
		}
	case tea.KeyEnter:
		if len(providers) > m.settingsCursor {
			id := providers[m.settingsCursor].ID
			_ = m.store.SetDefault(id)
			m.lines = append(m.lines, styleDim.Render("Set default provider to: "+providers[m.settingsCursor].Name))
		}
	case tea.KeyDelete, tea.KeyBackspace:
		if len(providers) > m.settingsCursor {
			id := providers[m.settingsCursor].ID
			_ = m.store.Remove(id)
			if m.settingsCursor > 0 {
				m.settingsCursor--
			}
		}
	}
	return m, nil
}

func (m *model) View() string {
	var b strings.Builder

	if m.view == viewSettings {
		return m.settingsView()
	}

	// Plan pane.
	if m.plan != nil && len(m.plan.Steps) > 0 {
		b.WriteString(stylePlan.Render("Plan:") + "\n")
		icons := map[orchestrator.StepStatus]string{
			orchestrator.StepPending: "[ ]", orchestrator.StepActive: "[>]",
			orchestrator.StepDone: "[x]", orchestrator.StepBlocked: "[!]",
		}
		for _, s := range m.plan.Steps {
			b.WriteString(stylePlan.Render(fmt.Sprintf("  %s %s. %s", icons[s.Status], s.ID, s.Title)) + "\n")
		}
		b.WriteString("\n")
	}

	// Transcript.
	start := 0
	if len(m.lines) > 200 {
		start = len(m.lines) - 200
	}
	for _, l := range m.lines[start:] {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n")

	// Prompt / modal.
	switch m.state {
	case stateApproval:
		tier := m.approval.tool.Tier()
		b.WriteString(styleWarn.Render(fmt.Sprintf(
			"APPROVAL REQUIRED  tier=T%d  tool=%s\n  %s\n  [y] allow once   [s] allow for session   [n] deny",
			tier, m.approval.tool.Name(), truncLine(m.approval.preview, 100))))
	case stateQuestion:
		q := m.question.qs[0]
		b.WriteString(styleWarn.Render("QUESTION: "+q.Prompt) + "\n")
		for i, o := range q.Options {
			b.WriteString(styleWarn.Render(fmt.Sprintf("  [%d] %s", i+1, o.Label)) + "\n")
		}
	case stateBusy:
		b.WriteString(styleTool.Render(fmt.Sprintf("… working  (in=%d out=%d tok)  /saver to toggle cost mode",
			m.inToks, m.outToks)))
	default:
		mode := "normal"
		if m.engine.Mode == orchestrator.ModeSaver {
			mode = "💰saver"
		}
		b.WriteString(styleUser.Render("> ") + m.input + "█" +
			styleDim.Render(fmt.Sprintf("  [%s] Tab=settings  /saver=toggle", mode)))
	}
	return b.String()
}

func (m *model) settingsView() string {
	var b strings.Builder
	b.WriteString(styleHead.Render("Settings — AI Providers") + "\n")
	b.WriteString(styleDim.Render("↑↓ navigate  Enter=set default  Del=remove  Tab=back to chat") + "\n\n")

	providers := m.store.List()
	if len(providers) == 0 {
		b.WriteString(styleDim.Render("No providers configured.") + "\n")
		b.WriteString(styleDim.Render("Run first-run setup or add via the web GUI (velkrod).") + "\n")
		return b.String()
	}
	for i, p := range providers {
		def := ""
		if p.IsDefault {
			def = " [default]"
		}
		line := fmt.Sprintf("  %s  %s · %s%s", p.Name, p.Kind, p.Model, def)
		if i == m.settingsCursor {
			b.WriteString(styleSel.Render("▶ "+line) + "\n")
		} else {
			b.WriteString(styleDim.Render("  "+line) + "\n")
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncLine(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
