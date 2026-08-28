package goconfig

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The test binary doubles as a fake $EDITOR: with GOCONFIG_TEST_EDITOR set it
// rewrites the config path passed as its last argv element ("break" writes
// invalid TOML) and exits.
func TestMain(m *testing.M) {
	switch os.Getenv("GOCONFIG_TEST_EDITOR") {
	case "":
		os.Exit(m.Run())
	case "break":
		os.WriteFile(os.Args[len(os.Args)-1], []byte("not [valid"), 0o600)
		os.Exit(0)
	default:
		os.WriteFile(os.Args[len(os.Args)-1], []byte("version = 3\n[server]\nport = 1234\n"), 0o600)
		os.Exit(0)
	}
}

func newTestCLI(t *testing.T) (*Manager, *bytes.Buffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	return New(Options{Path: path, Template: tmpl}), &bytes.Buffer{}, path
}

// run invokes m.run non-interactively (deterministic in tests: no TTY).
func run(m *Manager, args []string, out *bytes.Buffer) error {
	return m.run(args, out, false)
}

func TestCLISubcommands(t *testing.T) {
	m, out, _ := newTestCLI(t)

	if err := run(m, []string{"list"}, out); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"version = 1", "server.port = 8080", "server.host = localhost"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list missing %q:\n%s", want, out)
		}
	}

	out.Reset()
	if err := run(m, []string{"get", "server.port"}, out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "8080" {
		t.Errorf("get server.port = %q, want 8080", got)
	}

	if err := run(m, []string{"set", "server.port", "9999"}, out); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := run(m, []string{"set", "ui.theme", "dark"}, out); err != nil {
		t.Fatalf("set new key: %v", err)
	}
	doc, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := doc.Get("server.port"); v != int64(9999) {
		t.Errorf("server.port = %v, want 9999", v)
	}
	if v, _ := doc.Get("ui.theme"); v != "dark" {
		t.Errorf("ui.theme = %v (%T), want dark string", v, v)
	}

	// Type of an existing key must be kept: port stays integer.
	if err := run(m, []string{"set", "server.port", "abc"}, out); err == nil ||
		!strings.Contains(err.Error(), "not an integer") {
		t.Fatalf("want type error, got %v", err)
	}
	doc, _ = m.Load()
	if v, _ := doc.Get("server.port"); v != int64(9999) {
		t.Errorf("server.port changed on rejected set: %v", v)
	}

	if err := run(m, []string{"del", "server.host"}, out); err != nil {
		t.Fatalf("del: %v", err)
	}
	doc, _ = m.Load()
	if _, ok := doc.Get("server.host"); ok {
		t.Error("server.host still present after del")
	}
	if err := run(m, []string{"del", "server.host"}, out); err == nil {
		t.Error("double del should fail")
	}
}

func TestCLIErrors(t *testing.T) {
	m, out, _ := newTestCLI(t)
	if err := run(m, []string{"get", "nope"}, out); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("get missing key: %v", err)
	}
	if err := run(m, []string{"get", "server"}, out); err == nil || !strings.Contains(err.Error(), "table") {
		t.Errorf("get table: %v", err)
	}
	if err := run(m, []string{"bogus"}, out); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown command: %v", err)
	}
	if err := run(m, nil, out); err != nil {
		t.Errorf("no args should print usage: %v", err)
	}
	if !strings.Contains(out.String(), "usage") {
		t.Error("no args did not print usage")
	}
}

func TestCLIValidateOnSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := New(Options{Path: path, Template: tmpl, Validate: func(d *Doc) error {
		if p, ok := d.Get("server.port"); ok {
			if i, ok := p.(int64); ok && i > 10000 {
				return errors.New("port too high")
			}
		}
		return nil
	}})
	if err := run(m, []string{"set", "server.port", "65536"}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "port too high") {
		t.Fatalf("want validate error, got %v", err)
	}
	doc, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := doc.Get("server.port"); v != int64(8080) {
		t.Errorf("rejected set persisted: %v", v)
	}
}

func fakeEditor(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestEditRewritesViaEditor(t *testing.T) {
	m, out, path := newTestCLI(t)
	if _, err := m.Load(); err != nil { // create the file first
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", fakeEditor(t))
	t.Setenv("GOCONFIG_TEST_EDITOR", "rewrite")

	if err := run(m, []string{"--edit"}, out); err != nil {
		t.Fatalf("--edit: %v", err)
	}
	doc, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := doc.Get("server.port"); v != int64(1234) {
		t.Errorf("server.port = %v, want editor's 1234", v)
	}
	if bak := readDisk(t, path+".bak"); bak != tmpl {
		t.Errorf(".bak should hold the pre-edit file:\n%s", bak)
	}
}

func TestEditBrokenTomlKeepsFile(t *testing.T) {
	m, out, path := newTestCLI(t)
	if _, err := m.Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", fakeEditor(t))
	t.Setenv("GOCONFIG_TEST_EDITOR", "break")

	err := run(m, []string{"-edit"}, out)
	if err == nil || !strings.Contains(err.Error(), ".bak") {
		t.Fatalf("want parse error mentioning .bak, got %v", err)
	}
	if got := readDisk(t, path); got != "not [valid" {
		t.Errorf("user's broken file must be left as-is:\n%s", got)
	}
	if bak := readDisk(t, path+".bak"); bak != tmpl {
		t.Errorf(".bak should hold the pre-edit file:\n%s", bak)
	}
}

func TestResolveEditor(t *testing.T) {
	t.Setenv("VISUAL", "visual-exe -w")
	t.Setenv("EDITOR", "editor-exe")
	if got := resolveEditor(); got != "visual-exe -w" {
		t.Errorf("VISUAL should win: %q", got)
	}
	t.Setenv("VISUAL", "")
	if got := resolveEditor(); got != "editor-exe" {
		t.Errorf("EDITOR fallback: %q", got)
	}
	t.Setenv("EDITOR", "")
	want := "vi"
	if runtime.GOOS == "windows" {
		want = "notepad"
	}
	if got := resolveEditor(); got != want {
		t.Errorf("default editor = %q, want %q", got, want)
	}
}
