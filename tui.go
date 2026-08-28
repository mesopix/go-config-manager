package goconfig

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI runs the full-screen settings menu: arrow keys (or j/k) select a
// setting, enter edits it. Every confirmed edit is validated and persisted
// immediately (same semantics as `config set`), so quitting discards nothing.
func (m *Manager) TUI() error {
	doc, err := m.Load()
	if err != nil {
		return err
	}
	keys := scalarKeys(doc)
	if len(keys) == 0 {
		return fmt.Errorf("config %s: no editable settings", m.opts.Path)
	}
	p := tea.NewProgram(newTUIModel(m, doc, keys), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// scalarKeys returns the doc's leaf keys, excluding tables.
func scalarKeys(doc *Doc) []string {
	var keys []string
	for _, k := range doc.Keys() {
		if v, ok := doc.Get(k); ok {
			if _, isTable := v.(map[string]any); !isTable {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

type tuiModel struct {
	mgr     *Manager
	doc     *Doc
	keys    []string
	cursor  int
	editing bool
	input   textinput.Model
	err     string
	status  string
}

func newTUIModel(m *Manager, doc *Doc, keys []string) tuiModel {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 0
	in.Width = 60
	return tuiModel{mgr: m, doc: doc, keys: keys, input: in}
}

func (tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return m, nil
	}
	if m.editing {
		return m.updateEdit(key)
	}
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.keys)-1 {
			m.cursor++
		}
	case "enter", "l":
		m.editing = true
		m.err, m.status = "", ""
		m.input.Focus()
		v, _ := m.doc.Get(m.keys[m.cursor])
		m.input.SetValue(fmt.Sprintf("%v", v))
		return m, textinput.Blink
	}
	return m, nil
}

func (m tuiModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing, m.err = false, ""
		m.input.Blur()
		return m, nil
	case "enter":
		key := m.keys[m.cursor]
		v, err := m.mgr.applySet(m.doc, key, m.input.Value())
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.editing, m.err = false, ""
		m.input.Blur()
		m.status = fmt.Sprintf("saved %s = %v", key, v)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	if m.editing {
		hint := "[enter] save   [esc] cancel"
		if m.err != "" {
			hint = m.err + "\n  [enter] retry   [esc] cancel"
		}
		return fmt.Sprintf("\n  editing %s\n\n  new value: %s\n\n  %s\n",
			m.keys[m.cursor], m.input.View(), hint)
	}
	var b strings.Builder
	b.WriteString("\n  settings — ↑/↓ or j/k select · enter edit · q quit\n\n")
	for i, k := range m.keys {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		v, _ := m.doc.Get(k)
		fmt.Fprintf(&b, "%s %-32s %v\n", cursor, k, v)
	}
	if m.status != "" {
		fmt.Fprintf(&b, "\n  %s\n", m.status)
	}
	return b.String()
}
