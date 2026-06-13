package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rufatronics/velkrogo/internal/config"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/reasoning"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// The TUI implements orchestrator.Approver and reasoning.Asker by blocking the
// engine goroutine on a reply channel while the user answers a modal.

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
	prog      *tea.Program
	approvals chan approvalReq
	questions chan questionReq
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

type uiState int

const (
	stateInput uiState = iota
	stateBusy
	stateApproval
	stateQuestion
)

type model struct {
	engine   *orchestrator.Engine
	cfg      config.Config
	state    uiState
	input    string
	lines    []string
	plan     *orchestrator.Plan
	approval *approvalReq
	question *questionReq
	inToks   int
	outToks  int
	width    int
}

var (
	styleUser  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleAgent = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleTool  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stylePlan  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
)

func runTUI(engine *orchestrator.Engine, events chan orchestrator.Event, cfg config.Config) error {
	m := &model{engine: engine, cfg: cfg, lines: []string{
		"VelkroGo — " + modeLabel(cfg) + ". Type a task and press Enter. Ctrl+C quits.",
	}}
	p := tea.NewProgram(m)

	bridge := &uiBridge{prog: p}
	engine.Approver = bridge
	engine.Asker = bridge

	// Pump engine events into the TUI.
	go func() {
		for ev := range events {
			p.Send(ev)
		}
	}()

	_, err := p.Run()
	return err
}

func modeLabel(cfg config.Config) string {
	if cfg.SaverMode {
		return fmt.Sprintf("%s/%s (saver mode)", cfg.Provider.Name, cfg.Provider.Model)
	}
	return fmt.Sprintf("%s/%s", cfg.Provider.Name, cfg.Provider.Model)
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case orchestrator.Event:
		switch msg.Kind {
		case "text":
			m.lines = append(m.lines, styleAgent.Render(msg.Text))
		case "tool_start":
			m.lines = append(m.lines, styleTool.Render("→ "+msg.Tool+" "+msg.Text))
		case "tool_done":
			m.lines = append(m.lines, styleTool.Render("← "+msg.Tool+": "+firstLine(msg.Text)))
		case "plan":
			m.plan = msg.Plan
		case "usage":
			m.inToks += msg.InToks
			m.outToks += msg.OutToks
		case "error":
			m.lines = append(m.lines, styleErr.Render("error: "+msg.Text))
		}
		return m, nil

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

func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.state {
	case stateApproval:
		switch key.String() {
		case "y":
			m.approval.reply <- approvalResp{approved: true}
		case "s": // allow this tool for the whole session
			m.approval.reply <- approvalResp{approved: true, grant: &policy.Grant{Capability: m.approval.tool.Name(), Scope: "*"}}
		case "n", "esc":
			m.approval.reply <- approvalResp{approved: false}
		default:
			return m, nil
		}
		m.approval = nil
		m.state = stateBusy
		return m, nil

	case stateQuestion:
		q := m.question.qs[0]
		if n, err := strconv.Atoi(key.String()); err == nil && n >= 1 && n <= len(q.Options) {
			m.question.reply <- []reasoning.Answer{{Selected: []string{q.Options[n-1].Label}}}
			m.lines = append(m.lines, styleWarn.Render("you chose: "+q.Options[n-1].Label))
			m.question = nil
			m.state = stateBusy
		}
		return m, nil

	case stateInput:
		switch key.Type {
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input)
			if text == "" {
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
		case tea.KeyRunes, tea.KeySpace:
			m.input += string(key.Runes)
			if key.Type == tea.KeySpace {
				m.input += " "
			}
		}
		return m, nil
	}
	return m, nil
}

func (m *model) View() string {
	var b strings.Builder

	// Plan pane (Manus-style visible outline).
	if m.plan != nil && len(m.plan.Steps) > 0 {
		b.WriteString(stylePlan.Render("Plan:") + "\n")
		for _, s := range m.plan.Steps {
			mark := map[orchestrator.StepStatus]string{
				orchestrator.StepPending: "[ ]", orchestrator.StepActive: "[>]",
				orchestrator.StepDone: "[x]", orchestrator.StepBlocked: "[!]",
			}[s.Status]
			b.WriteString(stylePlan.Render(fmt.Sprintf("  %s %s. %s", mark, s.ID, s.Title)) + "\n")
		}
		b.WriteString("\n")
	}

	// Transcript (last N lines to stay lightweight).
	start := 0
	if len(m.lines) > 200 {
		start = len(m.lines) - 200
	}
	for _, l := range m.lines[start:] {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n")

	// Modals / prompt line.
	switch m.state {
	case stateApproval:
		b.WriteString(styleWarn.Render(fmt.Sprintf(
			"APPROVAL REQUIRED (tier T%d)\n  %s\n  [y] allow once   [s] allow for session   [n] deny",
			m.approval.tool.Tier(), m.approval.preview)))
	case stateQuestion:
		q := m.question.qs[0]
		b.WriteString(styleWarn.Render("QUESTION: "+q.Prompt) + "\n")
		for i, o := range q.Options {
			b.WriteString(styleWarn.Render(fmt.Sprintf("  [%d] %s", i+1, o.Label)) + "\n")
		}
	case stateBusy:
		b.WriteString(styleTool.Render("… working (tokens in/out: " +
			strconv.Itoa(m.inToks) + "/" + strconv.Itoa(m.outToks) + ")"))
	default:
		b.WriteString(styleUser.Render("> ") + m.input + "█")
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
