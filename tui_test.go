package goconfig

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// press feeds key messages into the model and returns its final state.
func press(m tuiModel, keys ...string) tuiModel {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(tuiModel)
	}
	return m
}

func newTUI(t *testing.T) (tuiModel, *Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	m := New(Options{Path: path, Template: tmpl})
	doc, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	return newTUIModel(m, doc, scalarKeys(doc)), m, path
}

// sorted scalar keys of tmpl: server.host, server.port, version.
func TestTUINavigation(t *testing.T) {
	m, _, _ := newTUI(t)
	if m.keys[0] != "server.host" || m.keys[1] != "server.port" || m.keys[2] != "version" {
		t.Fatalf("keys = %v", m.keys)
	}
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	m = press(m, "down", "j")
	if m.cursor != 2 {
		t.Errorf("cursor after down+j = %d, want 2", m.cursor)
	}
	m = press(m, "down") // clamp at end
	if m.cursor != 2 {
		t.Errorf("cursor past end = %d", m.cursor)
	}
	m = press(m, "up", "k", "up", "up") // clamp at start
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestTUIQuit(t *testing.T) {
	m, _, _ := newTUI(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q cmd did not produce tea.QuitMsg")
	}
	if _, ok := next.(tuiModel); !ok {
		t.Error("model type changed")
	}
}

func TestTUIEditSaves(t *testing.T) {
	m, mgr, path := newTUI(t)
	m = press(m, "down") // cursor on server.port
	m = press(m, "enter")
	if !m.editing || m.input.Value() != "8080" {
		t.Fatalf("edit mode = %v input = %q", m.editing, m.input.Value())
	}
	if !strings.Contains(m.View(), "editing server.port") {
		t.Errorf("view missing key:\n%s", m.View())
	}
	m.input.SetValue("9090")
	m = press(m, "enter")

	if m.editing {
		t.Error("still editing after save")
	}
	if !strings.Contains(m.status, "saved server.port = 9090") {
		t.Errorf("status = %q", m.status)
	}
	doc, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := doc.Get("server.port"); v != int64(9090) {
		t.Errorf("server.port = %v, want 9090", v)
	}
	if !strings.Contains(readDisk(t, path), "9090") {
		t.Error("edit not persisted")
	}
}

func TestTUIEditCancel(t *testing.T) {
	m, mgr, _ := newTUI(t)
	m = press(m, "down", "enter")
	m.input.SetValue("1")
	m = press(m, "esc")
	if m.editing {
		t.Error("esc should leave edit mode")
	}
	doc, _ := mgr.Load()
	if v, _ := doc.Get("server.port"); v != int64(8080) {
		t.Errorf("server.port = %v, want unchanged 8080", v)
	}
}

// A rejected value must not leak into a later edit's save.
func TestTUIValidationErrorNoLeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	mgr := New(Options{Path: path, Template: tmpl, Validate: func(d *Doc) error {
		if p, ok := d.Get("server.port"); ok {
			if i, ok := p.(int64); ok && i > 10000 {
				return errors.New("port too high")
			}
		}
		return nil
	}})
	doc, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(mgr, doc, scalarKeys(doc))

	m = press(m, "down", "enter") // server.port
	m.input.SetValue("65536")
	m = press(m, "enter")
	if !m.editing || !strings.Contains(m.err, "port too high") {
		t.Fatalf("want validation error staying in edit mode, err = %q", m.err)
	}
	if strings.Contains(readDisk(t, path), "65536") {
		t.Error("rejected value must not be persisted")
	}
	m = press(m, "esc", "up", "enter") // server.host
	m.input.SetValue("example.com")
	m = press(m, "enter")
	if m.err != "" || !strings.Contains(m.status, "saved server.host") {
		t.Fatalf("second edit failed: err = %q status = %q", m.err, m.status)
	}
	// The earlier rejected port must not ride along with this save.
	disk := readDisk(t, path)
	if !strings.Contains(disk, "example.com") || strings.Contains(disk, "65536") {
		t.Errorf("leak or missing save:\n%s", disk)
	}
	if v, _ := m.doc.Get("server.port"); v != int64(8080) {
		t.Errorf("server.port = %v, want 8080", v)
	}
}
