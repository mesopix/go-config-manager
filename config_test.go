package goconfig

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const tmpl = `# demo tool config
version = 1

[server]
port = 8080
host = "localhost"
`

func newManager(t *testing.T) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	return New(Options{Path: path, Template: tmpl}), path
}

func TestLoadCreatesFromTemplate(t *testing.T) {
	m, path := newManager(t)
	doc, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if string(got) != tmpl {
		t.Errorf("file content = %q, want template verbatim", got)
	}
	if v, ok := doc.Get("server.port"); !ok || v != int64(8080) {
		t.Errorf("server.port = %v (%T), want 8080", v, v)
	}
}

func TestLoadExistingFile(t *testing.T) {
	m, path := newManager(t)
	custom := "version = 2\n[server]\nport = 9000\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := doc.Get("server.port"); !ok || v != int64(9000) {
		t.Errorf("server.port = %v, want 9000", v)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	m, path := newManager(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not [valid toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Load(); err == nil {
		t.Error("want error for invalid TOML")
	}
}

func TestValidateRejects(t *testing.T) {
	m, path := newManager(t)
	m.opts.Validate = func(*Doc) error { return errors.New("boom") }
	if _, err := m.Load(); err == nil {
		t.Error("want validate error")
	}
	// The template file is still written on first run even if validation
	// then rejects it.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("template not written: %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	m, _ := newManager(t)
	doc, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	doc.Set("server.port", int64(9090))
	doc.Set("ui.theme", "dark")
	if err := m.Save(doc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := doc2.Get("server.port"); !ok || v != int64(9090) {
		t.Errorf("server.port = %v, want 9090", v)
	}
	if v, ok := doc2.Get("ui.theme"); !ok || v != "dark" {
		t.Errorf("ui.theme = %v, want dark", v)
	}
}

func TestDocDotPaths(t *testing.T) {
	doc, err := Parse([]byte("version = 1\n[server]\nport = 80\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Get("server.port"); !ok {
		t.Fatal("server.port missing")
	}
	if _, ok := doc.Get("server.nope"); ok {
		t.Error("server.nope should be absent")
	}
	if _, ok := doc.Get("version.sub"); ok {
		t.Error("dot path through non-table should be absent")
	}
	doc.Set("a.b.c", 1)
	if v, ok := doc.Get("a.b.c"); !ok || v != 1 {
		t.Errorf("a.b.c = %v, want 1", v)
	}
	doc.Set("version", 2)
	if v, _ := doc.Get("version"); v != 2 {
		t.Errorf("version = %v, want 2", v)
	}
	if !doc.Delete("server.port") {
		t.Error("Delete server.port = false, want true")
	}
	if _, ok := doc.Get("server.port"); ok {
		t.Error("server.port still present after Delete")
	}
	if doc.Delete("server.port") {
		t.Error("double Delete = true, want false")
	}
	if doc.Delete("x.y.z") {
		t.Error("Delete of unknown path = true, want false")
	}
}
