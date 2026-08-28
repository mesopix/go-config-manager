package goconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const v1Cfg = `version = 1
name = "demo"

[server]
port = 8080
`

// testMigrations: v1->v2 adds server.host, v2->v3 drops name, adds log.level.
func testMigrations() []Migration {
	return []Migration{
		{From: 1, Migrate: func(d *Doc) error {
			d.Set("server.host", "localhost")
			return nil
		}},
		{From: 2, Migrate: func(d *Doc) error {
			d.Delete("name")
			d.Set("log.level", "info")
			return nil
		}},
	}
}

func writeCfg(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readDisk(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMigrateChainFromV1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, v1Cfg)
	m := New(Options{Path: path, Template: "version = 3\n", Version: 3, Migrations: testMigrations()})

	doc, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := doc.Version(); !ok || v != 3 {
		t.Errorf("version = %v %v, want 3", v, ok)
	}
	// Untouched fields must survive the whole chain.
	if v, ok := doc.Get("server.port"); !ok || v != int64(8080) {
		t.Errorf("server.port = %v, want 8080 (untouched field must survive)", v)
	}
	if v, _ := doc.Get("server.host"); v != "localhost" {
		t.Errorf("server.host = %v, want localhost", v)
	}
	if v, _ := doc.Get("log.level"); v != "info" {
		t.Errorf("log.level = %v, want info", v)
	}
	if _, ok := doc.Get("name"); ok {
		t.Error("name should be removed by v2->v3")
	}
	if !strings.Contains(readDisk(t, path), "version = 3") {
		t.Errorf("disk file not migrated:\n%s", readDisk(t, path))
	}
	if bak := readDisk(t, path+".bak"); bak != v1Cfg {
		t.Errorf(".bak does not hold the original v1 content:\n%s", bak)
	}
}

func TestMigrateFromV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, "version = 2\n[server]\nport = 9000\nhost = \"example.com\"\n")
	m := New(Options{Path: path, Template: "version = 3\n", Version: 3, Migrations: testMigrations()})

	doc, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, _ := doc.Version(); v != 3 {
		t.Errorf("version = %v, want 3", v)
	}
	// v1->v2 migration must not run for a v2 document.
	if v, _ := doc.Get("server.host"); v != "example.com" {
		t.Errorf("server.host = %v, want example.com (v1->v2 must not run)", v)
	}
	if v, _ := doc.Get("log.level"); v != "info" {
		t.Errorf("log.level = %v, want info", v)
	}
}

func TestNoMigrationWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	v3 := "version = 3\n[server]\nport = 1\n"
	writeCfg(t, path, v3)
	m := New(Options{Path: path, Template: v3, Version: 3, Migrations: testMigrations()})

	if _, err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := readDisk(t, path); got != v3 {
		t.Errorf("file rewritten without migration:\n%s", got)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unexpected .bak: %v", err)
	}
}

func TestNewerVersionRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, "version = 5\n")
	m := New(Options{Path: path, Template: "version = 3\n", Version: 3, Migrations: testMigrations()})

	_, err := m.Load()
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("want newer-version error, got %v", err)
	}
	if got := readDisk(t, path); !strings.Contains(got, "version = 5") {
		t.Errorf("rejected file must stay untouched:\n%s", got)
	}
}

func TestBrokenChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, v1Cfg)
	m := New(Options{Path: path, Template: "version = 3\n", Version: 3,
		Migrations: []Migration{testMigrations()[0]}}) // only v1->v2

	_, err := m.Load()
	if err == nil || !strings.Contains(err.Error(), "from schema version 2") {
		t.Fatalf("want broken-chain error, got %v", err)
	}
	if got := readDisk(t, path); got != v1Cfg {
		t.Errorf("file must stay at v1 on broken chain:\n%s", got)
	}
}

func TestMissingVersionField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, "[server]\nport = 1\n")
	m := New(Options{Path: path, Template: "version = 3\n", Version: 3, Migrations: testMigrations()})

	if _, err := m.Load(); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("want missing-version error, got %v", err)
	}
}

func TestMigrationErrorLeavesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, v1Cfg)
	boom := errors.New("boom")
	m := New(Options{Path: path, Template: "version = 2\n", Version: 2,
		Migrations: []Migration{{From: 1, Migrate: func(*Doc) error { return boom }}}})

	if _, err := m.Load(); !errors.Is(err, boom) {
		t.Fatalf("want migration error, got %v", err)
	}
	if got := readDisk(t, path); got != v1Cfg {
		t.Errorf("file must stay untouched on migration error:\n%s", got)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("no .bak may be written before a successful migration: %v", err)
	}
}

func TestValidateAfterMigrationBeforePersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, v1Cfg)
	m := New(Options{
		Path: path, Template: "version = 3\n", Version: 3, Migrations: testMigrations(),
		Validate: func(*Doc) error { return errors.New("bad schema") },
	})

	_, err := m.Load()
	if err == nil || !strings.Contains(err.Error(), "invalid after migration") {
		t.Fatalf("want post-migration validation error, got %v", err)
	}
	// Rejected migration must not persist anything.
	if got := readDisk(t, path); got != v1Cfg {
		t.Errorf("file must stay at v1 when migration result is rejected:\n%s", got)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("no .bak for rejected migration: %v", err)
	}
}

func TestFreshTemplateOldVersionMigrates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	m := New(Options{Path: path, Template: v1Cfg, Version: 3, Migrations: testMigrations()})

	doc, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, _ := doc.Version(); v != 3 {
		t.Errorf("version = %v, want 3", v)
	}
	if !strings.Contains(readDisk(t, path), "version = 3") {
		t.Error("fresh file not migrated")
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("fresh install must not leave a .bak: %v", err)
	}
}

func TestVersionZeroDisablesVersioning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeCfg(t, path, "[server]\nport = 1\n") // no version field, no Version set
	m := New(Options{Path: path, Template: "port = 1\n"})

	if _, err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
