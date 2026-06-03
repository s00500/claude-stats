package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"claude-stats/internal/stats"
)

type sortMode int

const (
	sortDate sortMode = iota
	sortName
	sortAge
	sortProject
)

func (m sortMode) String() string {
	switch m {
	case sortDate:
		return "date"
	case sortName:
		return "name"
	case sortAge:
		return "age"
	case sortProject:
		return "project"
	}
	return "?"
}

type sessionItem struct {
	s stats.SessionInfo
}

func (i sessionItem) Title() string {
	proj := filepath.Base(i.s.Project)
	if proj == "" || proj == "." || proj == "/" {
		proj = i.s.Project
	}
	when := i.s.StartTime
	if when == "" {
		when = "unknown"
	}
	return fmt.Sprintf("%s  %s", when, proj)
}

func (i sessionItem) Description() string {
	parts := []string{
		fmt.Sprintf("%dm", i.s.MessageCount),
		fmt.Sprintf("%dt", i.s.ToolCallCount),
	}
	if i.s.Model != "" {
		parts = append(parts, i.s.Model)
	}
	if i.s.Title != "" {
		parts = append(parts, `"`+i.s.Title+`"`)
	}
	if age := humanAge(i.s.LastActivity); age != "" {
		parts = append(parts, "("+age+")")
	}
	return strings.Join(parts, "  ")
}

func (i sessionItem) FilterValue() string {
	return strings.Join([]string{
		i.s.Project, i.s.Title, i.s.SessionID, i.s.Model,
	}, " ")
}

type model struct {
	list     list.Model
	sessions []stats.SessionInfo
	sort     sortMode
	reversed bool
	chosen   *stats.SessionInfo
}

var (
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginLeft(2)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginLeft(2)
)

// Run launches the interactive browser. On Enter, the selected session is
// returned. On q/Esc, returns nil with no error.
func Run(sessions []stats.SessionInfo) (*stats.SessionInfo, error) {
	m := newModel(sessions)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}
	final, ok := result.(model)
	if !ok {
		return nil, nil
	}
	return final.chosen, nil
}

func newModel(sessions []stats.SessionInfo) model {
	m := model{sessions: append([]stats.SessionInfo(nil), sessions...), sort: sortDate}
	sortSessions(m.sessions, m.sort, m.reversed)

	delegate := list.NewDefaultDelegate()
	l := list.New(toItems(m.sessions), delegate, 0, 0)
	l.Title = "claude-stats sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	m.list = l
	return m
}

func toItems(sessions []stats.SessionInfo) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{s: s}
	}
	return items
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := lipgloss.NewStyle().Margin(1, 2).GetFrameSize()
		// Reserve two extra lines for our footer.
		m.list.SetSize(msg.Width-h, msg.Height-v-2)
	case tea.KeyMsg:
		// While the bubbles list is in filter-input mode, let it consume keys
		// so users can actually type slashes, q, etc. without us hijacking.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				s := it.s
				m.chosen = &s
				return m, tea.Quit
			}
		case "d":
			m = m.setSort(sortDate)
			return m, nil
		case "n":
			m = m.setSort(sortName)
			return m, nil
		case "a":
			m = m.setSort(sortAge)
			return m, nil
		case "p":
			m = m.setSort(sortProject)
			return m, nil
		case "r":
			m.reversed = !m.reversed
			sortSessions(m.sessions, m.sort, m.reversed)
			m.list.SetItems(toItems(m.sessions))
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) setSort(mode sortMode) model {
	if m.sort == mode {
		m.reversed = !m.reversed
	} else {
		m.sort = mode
		m.reversed = false
	}
	sortSessions(m.sessions, m.sort, m.reversed)
	m.list.SetItems(toItems(m.sessions))
	return m
}

func (m model) View() string {
	arrow := "↓"
	if m.reversed {
		arrow = "↑"
	}
	status := fmt.Sprintf("%d sessions · sort: %s%s", len(m.sessions), m.sort, arrow)
	help := "[d]ate  [n]ame  [a]ge  [p]roject  [r]everse  [/] filter  [enter] open  [q] quit"
	return lipgloss.JoinVertical(lipgloss.Left,
		m.list.View(),
		statusStyle.Render(status),
		helpStyle.Render(help),
	)
}

func sortSessions(s []stats.SessionInfo, mode sortMode, reversed bool) {
	less := func(i, j int) bool {
		switch mode {
		case sortName:
			a := s[i].Title
			b := s[j].Title
			if a == "" {
				a = s[i].SessionID
			}
			if b == "" {
				b = s[j].SessionID
			}
			return strings.ToLower(a) < strings.ToLower(b)
		case sortProject:
			return strings.ToLower(s[i].Project) < strings.ToLower(s[j].Project)
		case sortAge:
			// Newest first by default — same key as date.
			fallthrough
		case sortDate:
			fallthrough
		default:
			return s[i].StartTime > s[j].StartTime
		}
	}
	sort.SliceStable(s, less)
	if reversed {
		for l, r := 0, len(s)-1; l < r; l, r = l+1, r-1 {
			s[l], s[r] = s[r], s[l]
		}
	}
}

// RenderDetail formats a single session as a multi-line detail block suitable
// for stdout.
func RenderDetail(s stats.SessionInfo) string {
	sid := s.SessionID
	if len(sid) > 8 {
		sid = sid[:8] + "…"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session %s\n", sid)
	fmt.Fprintf(&b, "  Full ID:       %s\n", s.SessionID)
	fmt.Fprintf(&b, "  Project:       %s\n", s.Project)
	if s.Model != "" {
		fmt.Fprintf(&b, "  Model:         %s\n", s.Model)
	}
	if s.Title != "" {
		fmt.Fprintf(&b, "  Title:         %s\n", s.Title)
	}
	if s.StartTime != "" {
		fmt.Fprintf(&b, "  Started:       %s\n", s.StartTime)
	}
	if s.LastActivity != "" {
		age := humanAge(s.LastActivity)
		if age != "" {
			fmt.Fprintf(&b, "  Last activity: %s (%s)\n", s.LastActivity, age)
		} else {
			fmt.Fprintf(&b, "  Last activity: %s\n", s.LastActivity)
		}
	}
	fmt.Fprintf(&b, "  Messages:      %d\n", s.MessageCount)
	fmt.Fprintf(&b, "  Tool calls:    %d\n", s.ToolCallCount)
	fmt.Fprintf(&b, "  Size:          %s\n", humanBytes(s.SizeBytes))
	if s.Version != "" {
		fmt.Fprintf(&b, "  Version:       %s\n", s.Version)
	}
	return b.String()
}

func humanAge(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", ts, time.Local)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

func humanBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
