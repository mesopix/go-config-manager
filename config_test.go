package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempConfigDir 将 os.UserConfigDir 指向一个全新的临时目录，
// 避免测试触碰真实用户配置。
func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS
}

// ---------- load() 正例 ----------

func TestLoad_validJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := []byte(`{"meta": {"version": "1.0.0"}, "fields": {"name": "demo", "port": 8080, "debug": true}}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := load(path)
	if err != nil {
		t.Fatalf("load valid JSON: unexpected error: %v", err)
	}

	if name, ok := c.Get("name"); !ok || name != "demo" {
		t.Errorf("name = %v, %v; want demo, true", name, ok)
	}
	if port, ok := c.Get("port"); !ok || port != float64(8080) {
		t.Errorf("port = %v, %v; want 8080, true", port, ok)
	}
	if debug, ok := c.Get("debug"); !ok || debug != true {
		t.Errorf("debug = %v, %v; want true, true", debug, ok)
	}
	if c.DeclaredVersion() != "1.0.0" {
		t.Errorf("DeclaredVersion = %q, want 1.0.0", c.DeclaredVersion())
	}
}

// ---------- load() 反例（表驱动） ----------

func TestLoad_invalidJSON(t *testing.T) {
	tests := []struct {
		label   string
		content string
	}{
		{"syntax error", `{invalid}`},
		{"truncated", `{"key": "val`},
		{"empty file", ``},
		{"array instead of object", `[1, 2, 3]`},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := load(path)
			if err == nil {
				t.Errorf("load(%q): expected error, got nil", tt.label)
			}
		})
	}
}

// ---------- load() 文件不存在 ----------

func TestLoad_fileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	_, err := load(path)
	if err == nil {
		t.Fatal("load nonexistent file: expected error, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("load nonexistent file: error = %v; want os.IsNotExist", err)
	}
}

// ---------- Save + load 往返 ----------

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	c := &Config{path: path, data: map[string]any{"meta": map[string]any{}, "fields": map[string]any{}}, resolvedVersion: UnknownVersion}
	c.Set("name", "demo")
	c.Set("port", 8080)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := got.Get("name"); !ok || name != "demo" {
		t.Fatalf("name = %v, %v", name, ok)
	}
	if port, ok := got.Get("port"); !ok || port != float64(8080) { // JSON 数值是 float64
		t.Fatalf("port = %v, %v", port, ok)
	}
}

// ---------- 目录权限 ----------

func TestLoadAppConfigDirPermissions(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {"schema_version": "1.0.0"}, "fields": {"key": "value"}}`)
	if _, err := LoadAppConfig("permtest", defaultJSON); err != nil {
		t.Fatal(err)
	}

	// 验证配置目录权限至少为 0700。
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "permtest")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&configDirMode != configDirMode {
		t.Fatalf("dir permissions = %o, want at least %o", perm, configDirMode)
	}
}

// ---------- 原子保存 ----------

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	c := &Config{path: path, data: map[string]any{"meta": map[string]any{}, "fields": map[string]any{}}, resolvedVersion: UnknownVersion}
	c.Set("key", "value")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// 验证原子保存后内容正确。
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if val, ok := got.Get("key"); !ok || val != "value" {
		t.Fatalf("key = %v, %v; want value, true", val, ok)
	}

	// 验证文件权限至少为 0600（屏蔽 Unix 上 umask 的影响）。
	// 注意：Windows 上文件权限模型不同，此断言仅在 Unix 上有意义。
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&configFileMode != configFileMode {
		t.Fatalf("permissions = %o, want at least %o", perm, configFileMode)
	}

	// 多次保存并验证没有残留的临时文件。
	for range 5 {
		c.Set("counter", float64(1))
		if err := c.Save(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("leftover temp file found: %s", entry.Name())
		}
	}
}
