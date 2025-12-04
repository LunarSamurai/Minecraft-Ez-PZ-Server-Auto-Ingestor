package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcserver-manager/internal/server"
	"mcserver-manager/internal/stats"
)

var (
	primaryColor = lipgloss.Color("#7C3AED")
	errorColor   = lipgloss.Color("#EF4444")
	warningColor = lipgloss.Color("#F59E0B")
	successColor = lipgloss.Color("#10B981")
	textColor    = lipgloss.Color("#E5E7EB")
	dimColor     = lipgloss.Color("#6B7280")
	borderColor  = lipgloss.Color("#374151")
)

var dimStyle = lipgloss.NewStyle().Foreground(dimColor)
var valueStyle = lipgloss.NewStyle().Foreground(textColor).Bold(true)
var headerStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
var playerOnlineStyle = lipgloss.NewStyle().Foreground(successColor)

var serverCommands = []string{
	"list - List players",
	"say <msg> - Broadcast",
	"kick <player>",
	"ban <player>",
	"op <player>",
	"tp <p> <x> <y> <z>",
	"give <p> <item>",
	"time set <val>",
	"weather <type>",
	"save-all",
	"stop",
}

type Model struct {
	config      *server.Config
	srv         *server.Server
	serverStats server.ServerStats

	consoleViewport viewport.Model
	playerViewport  viewport.Model
	commandInput    textinput.Model
	consoleLines    []string

	width  int
	height int

	ready        bool
	inputFocused bool
	quitting     bool
	focusPanel   int // 0=console, 1=players

	tpsHistory    []float64
	memoryHistory []float64
	cpuHistory    []float64

	playerEvents []PlayerEvent
}

type PlayerEvent struct {
	Time    time.Time
	Player  string
	Type    string
	Message string
}

type tickMsg time.Time

func Run(config *server.Config) error {
	m := NewModel(config)
	p := tea.NewProgram(m, tea.WithAltScreen())

	m.srv = server.New(config)
	go func() {
		m.srv.Start()
	}()

	_, err := p.Run()

	if m.srv != nil {
		m.srv.Stop()
	}

	return err
}

func NewModel(config *server.Config) *Model {
	ti := textinput.New()
	ti.Placeholder = "Enter command..."
	ti.CharLimit = 256
	ti.Width = 60

	vp := viewport.New(80, 20)
	playerVp := viewport.New(30, 10)

	return &Model{
		config:          config,
		consoleViewport: vp,
		playerViewport:  playerVp,
		commandInput:    ti,
		consoleLines:    make([]string, 0, 1000),
		tpsHistory:      make([]float64, 0, 60),
		memoryHistory:   make([]float64, 0, 60),
		cpuHistory:      make([]float64, 0, 60),
		playerEvents:    make([]PlayerEvent, 0, 100),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if !m.inputFocused {
				m.quitting = true
				return m, tea.Quit
			}
		case "tab":
			m.inputFocused = !m.inputFocused
			if m.inputFocused {
				m.commandInput.Focus()
			} else {
				m.commandInput.Blur()
			}
		case "enter":
			if m.inputFocused && m.commandInput.Value() != "" {
				cmd := m.commandInput.Value()
				m.commandInput.Reset()
				if m.srv != nil {
					m.srv.SendCommand(cmd)
				}
			}
		case "r":
			if !m.inputFocused && m.srv != nil {
				go m.srv.Restart()
			}
		case "s":
			if !m.inputFocused && m.srv != nil {
				if m.serverStats.Status == server.StatusRunning {
					go m.srv.Stop()
				} else if m.serverStats.Status == server.StatusStopped {
					go m.srv.Start()
				}
			}
		case "left", "right":
			if !m.inputFocused && m.showSidePanel() {
				m.focusPanel = (m.focusPanel + 1) % 2
			}
		case "up", "k":
			if !m.inputFocused {
				if m.focusPanel == 0 || !m.showSidePanel() {
					m.consoleViewport.LineUp(1)
				} else {
					m.playerViewport.LineUp(1)
				}
			}
		case "down", "j":
			if !m.inputFocused {
				if m.focusPanel == 0 || !m.showSidePanel() {
					m.consoleViewport.LineDown(1)
				} else {
					m.playerViewport.LineDown(1)
				}
			}
		case "pgup":
			if !m.inputFocused {
				m.consoleViewport.HalfViewUp()
			}
		case "pgdown":
			if !m.inputFocused {
				m.consoleViewport.HalfViewDown()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.recalculateLayout()

	case tickMsg:
		if m.srv != nil {
			m.serverStats = m.srv.GetStats()

			m.tpsHistory = append(m.tpsHistory, m.serverStats.TPS)
			if len(m.tpsHistory) > 60 {
				m.tpsHistory = m.tpsHistory[1:]
			}

			memPercent := 0.0
			if m.serverStats.MemoryMax > 0 {
				memPercent = float64(m.serverStats.MemoryUsed) / float64(m.serverStats.MemoryMax) * 100
			}
			m.memoryHistory = append(m.memoryHistory, memPercent)
			if len(m.memoryHistory) > 60 {
				m.memoryHistory = m.memoryHistory[1:]
			}

			m.cpuHistory = append(m.cpuHistory, m.serverStats.CPUPercent)
			if len(m.cpuHistory) > 60 {
				m.cpuHistory = m.cpuHistory[1:]
			}

			select {
			case line := <-m.srv.OutputChan():
				m.consoleLines = append(m.consoleLines, line)
				if len(m.consoleLines) > 1000 {
					m.consoleLines = m.consoleLines[1:]
				}
				m.consoleViewport.SetContent(strings.Join(m.consoleLines, "\n"))
				m.consoleViewport.GotoBottom()
				m.parsePlayerEvent(line)
			default:
			}

			m.playerViewport.SetContent(m.renderPlayerPanel())
		}

		cmds = append(cmds, tickCmd())
	}

	if m.inputFocused {
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.consoleViewport, cmd = m.consoleViewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// showSidePanel returns true if there's enough width for the side panel
func (m *Model) showSidePanel() bool {
	return m.width >= 80
}

// recalculateLayout adjusts all viewport sizes based on current terminal size
func (m *Model) recalculateLayout() {
	// Reserve space: 1 status + 1 input + 1 help + 2 borders = 5 lines minimum
	panelHeight := m.height - 5
	if panelHeight < 5 {
		panelHeight = 5
	}

	if m.showSidePanel() {
		// Two-panel mode: 70% console, 30% side panel
		rightWidth := m.width * 30 / 100
		if rightWidth < 20 {
			rightWidth = 20
		}
		if rightWidth > 35 {
			rightWidth = 35
		}
		leftWidth := m.width - rightWidth - 3 // 3 for gap and borders

		m.consoleViewport.Width = leftWidth - 2
		m.consoleViewport.Height = panelHeight - 2
		m.playerViewport.Width = rightWidth - 2
		m.playerViewport.Height = panelHeight - 2
	} else {
		// Single-panel mode: console only
		m.consoleViewport.Width = m.width - 4
		m.consoleViewport.Height = panelHeight - 2
	}

	m.commandInput.Width = m.width - 4
}

func (m *Model) parsePlayerEvent(line string) {
	lowerLine := strings.ToLower(line)

	if strings.Contains(line, "joined the game") {
		name := extractPlayerName(line)
		m.addPlayerEvent(name, "join", "Joined")
	} else if strings.Contains(line, "left the game") {
		name := extractPlayerName(line)
		m.addPlayerEvent(name, "leave", "Left")
	} else if strings.Contains(lowerLine, "was slain") || strings.Contains(lowerLine, "died") ||
		strings.Contains(lowerLine, "was killed") || strings.Contains(lowerLine, "drowned") ||
		strings.Contains(lowerLine, "burned") || strings.Contains(lowerLine, "fell") {
		name := extractPlayerName(line)
		m.addPlayerEvent(name, "death", "Died")
	}
}

func extractPlayerName(text string) string {
	if idx := strings.LastIndex(text, "]: "); idx != -1 {
		rest := text[idx+3:]
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "Player"
}

func (m *Model) addPlayerEvent(player, eventType, message string) {
	event := PlayerEvent{
		Time:    time.Now(),
		Player:  player,
		Type:    eventType,
		Message: message,
	}
	m.playerEvents = append(m.playerEvents, event)
	if len(m.playerEvents) > 50 {
		m.playerEvents = m.playerEvents[1:]
	}
}

func (m *Model) renderPlayerPanel() string {
	var b strings.Builder
	panelWidth := m.playerViewport.Width

	// Players section
	header := fmt.Sprintf("👥 PLAYERS %d/%d", m.serverStats.PlayerCount, m.serverStats.MaxPlayers)
	b.WriteString(headerStyle.Render(header) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", panelWidth)) + "\n")

	if len(m.serverStats.Players) == 0 {
		b.WriteString(dimStyle.Render("No players online\n"))
	} else {
		for _, player := range m.serverStats.Players {
			pt := time.Since(player.JoinedAt)
			line := fmt.Sprintf("● %s (%s)", player.Name, stats.FormatDurationShort(pt))
			b.WriteString(playerOnlineStyle.Render(line) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(headerStyle.Render("📋 EVENTS") + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", panelWidth)) + "\n")

	// Show as many events as will fit
	maxEvents := (m.playerViewport.Height - 10) / 1
	if maxEvents < 3 {
		maxEvents = 3
	}
	if maxEvents > 10 {
		maxEvents = 10
	}

	startIdx := len(m.playerEvents) - maxEvents
	if startIdx < 0 {
		startIdx = 0
	}

	if len(m.playerEvents) == 0 {
		b.WriteString(dimStyle.Render("No events yet\n"))
	} else {
		for _, ev := range m.playerEvents[startIdx:] {
			icon := "•"
			style := dimStyle
			switch ev.Type {
			case "join":
				icon = "→"
				style = lipgloss.NewStyle().Foreground(successColor)
			case "leave":
				icon = "←"
				style = lipgloss.NewStyle().Foreground(errorColor)
			case "death":
				icon = "☠"
				style = lipgloss.NewStyle().Foreground(warningColor)
			}
			timeStr := ev.Time.Format("15:04")
			b.WriteString(dimStyle.Render(timeStr+" ") + style.Render(icon+" "+ev.Player) + "\n")
		}
	}

	// Commands section - only if there's room
	remainingHeight := m.playerViewport.Height - strings.Count(b.String(), "\n") - 3
	if remainingHeight > 4 {
		b.WriteString("\n")
		b.WriteString(headerStyle.Render("⌨ COMMANDS") + "\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", panelWidth)) + "\n")

		cmdCount := remainingHeight - 1
		if cmdCount > len(serverCommands) {
			cmdCount = len(serverCommands)
		}
		for i := 0; i < cmdCount; i++ {
			b.WriteString(dimStyle.Render(serverCommands[i]) + "\n")
		}
	}

	return b.String()
}

func (m *Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	if m.quitting {
		return "Shutting down...\n"
	}

	// Recalculate on every render to stay responsive
	m.recalculateLayout()

	var b strings.Builder

	// Status bar - adapts to width
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Main content area
	m.consoleViewport.SetContent(strings.Join(m.consoleLines, "\n"))

	if m.showSidePanel() {
		// Two-panel layout
		leftBorderColor := borderColor
		if m.focusPanel == 0 {
			leftBorderColor = primaryColor
		}
		consoleStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(leftBorderColor).
			Width(m.consoleViewport.Width + 2).
			Height(m.consoleViewport.Height + 2)

		rightBorderColor := borderColor
		if m.focusPanel == 1 {
			rightBorderColor = primaryColor
		}
		rightStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(rightBorderColor).
			Width(m.playerViewport.Width + 2).
			Height(m.playerViewport.Height + 2)

		consoleBox := consoleStyle.Render(m.consoleViewport.View())
		rightBox := rightStyle.Render(m.playerViewport.View())

		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, consoleBox, " ", rightBox))
	} else {
		// Single-panel layout
		consoleStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Width(m.consoleViewport.Width + 2).
			Height(m.consoleViewport.Height + 2)

		b.WriteString(consoleStyle.Render(m.consoleViewport.View()))
	}
	b.WriteString("\n")

	// Input line
	prefix := dimStyle.Render("> ")
	if m.inputFocused {
		prefix = lipgloss.NewStyle().Foreground(primaryColor).Render("> ")
	}
	b.WriteString(prefix + m.commandInput.View() + "\n")

	// Help line - adapts to width
	b.WriteString(m.renderHelpLine())

	return b.String()
}

func (m *Model) renderStatusBar() string {
	statusIcon := "⭕"
	statusText := "STOP"
	statusColor := errorColor
	switch m.serverStats.Status {
	case server.StatusRunning:
		statusIcon = "🟢"
		statusText = "RUN"
		statusColor = successColor
	case server.StatusStarting:
		statusIcon = "🟡"
		statusText = "STARTING"
		statusColor = warningColor
	case server.StatusRestarting:
		statusIcon = "🟡"
		statusText = "RESTART"
		statusColor = warningColor
	case server.StatusStopping:
		statusIcon = "🟡"
		statusText = "STOPPING"
		statusColor = warningColor
	case server.StatusCrashed:
		statusIcon = "🔴"
		statusText = "CRASH"
		statusColor = errorColor
	}

	statusStyle := lipgloss.NewStyle().Foreground(statusColor).Bold(true)
	tpsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stats.TPSColor(m.serverStats.TPS))).Bold(true)

	memPct := 0.0
	if m.serverStats.MemoryMax > 0 {
		memPct = float64(m.serverStats.MemoryUsed) / float64(m.serverStats.MemoryMax) * 100
	}
	memStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stats.MemoryColor(memPct)))
	cpuStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stats.CPUColor(m.serverStats.CPUPercent)))

	// Compact vs expanded based on width
	if m.width < 60 {
		// Ultra compact
		return fmt.Sprintf("%s%s T:%.0f M:%.0f%% P:%d",
			statusIcon,
			statusStyle.Render(statusText[:1]),
			m.serverStats.TPS,
			memPct,
			m.serverStats.PlayerCount,
		)
	} else if m.width < 90 {
		// Compact
		return fmt.Sprintf("%s %s │ TPS:%s │ Mem:%s │ P:%d/%d",
			statusIcon,
			statusStyle.Render(statusText),
			tpsStyle.Render(fmt.Sprintf("%.1f", m.serverStats.TPS)),
			memStyle.Render(fmt.Sprintf("%.0f%%", memPct)),
			m.serverStats.PlayerCount,
			m.serverStats.MaxPlayers,
		)
	} else {
		// Full
		return fmt.Sprintf("%s %s │ TPS: %s │ Mem: %s │ CPU: %s │ Players: %d/%d │ Uptime: %s",
			statusIcon,
			statusStyle.Render(statusText),
			tpsStyle.Render(fmt.Sprintf("%.1f", m.serverStats.TPS)),
			memStyle.Render(fmt.Sprintf("%.0f%%", memPct)),
			cpuStyle.Render(fmt.Sprintf("%.0f%%", m.serverStats.CPUPercent)),
			m.serverStats.PlayerCount,
			m.serverStats.MaxPlayers,
			valueStyle.Render(stats.FormatDurationShort(m.serverStats.Uptime)),
		)
	}
}

func (m *Model) renderHelpLine() string {
	if m.width < 50 {
		return dimStyle.Render("[Tab]In [R]Rst [Q]Quit")
	} else if m.width < 80 {
		return dimStyle.Render("[Tab]Input [↑↓]Scroll [R]Restart [S]Stop [Q]Quit")
	} else {
		return dimStyle.Render("[Tab]Input [←→]Panel [↑↓/PgUp/PgDn]Scroll [R]Restart [S]Start/Stop [Q]Quit")
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
