package configmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	c := &Config{path: path, data: map[string]any{}}
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
	if port, ok := got.Get("port"); !ok || port != float64(8080) { // JSON numbers are float64
		t.Fatalf("port = %v, %v", port, ok)
	}
}

func TestLoadAppConfigCreatesFromTemplate(t *testing.T) {
	useTempConfigDir(t)

	tpl := map[string]any{"name": "demo", "port": 8080}
	c, err := LoadAppConfig("myapp", tpl)
	if err != nil {
		t.Fatal(err)
	}

	if name, ok := c.Get("name"); !ok || name != "demo" {
		t.Fatalf("name = %v, %v", name, ok)
	}
	if port, ok := c.Get("port"); !ok || port != float64(8080) {
		t.Fatalf("port = %v, %v", port, ok)
	}

	// A second load reads from disk and keeps changes.
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadAppConfig("myapp", tpl)
	if err != nil {
		t.Fatal(err)
	}
	if port, ok := c2.Get("port"); !ok || port != float64(9090) {
		t.Fatalf("port = %v, %v", port, ok)
	}
}

// 演示上一轮讨论的坑：模板里的 8080 在内存中是 int，但首次运行也走
// "写盘→读回"，所以首末两次运行取出的类型一致，都是 float64。
// 如果首次运行跳过读回，这里第一次就会拿到 int，第二次拿到 float64。
func TestLoadAppConfigNumbersAreFloat64(t *testing.T) {
	useTempConfigDir(t)

	tpl := map[string]any{"port": 8080} // 内存里是 int

	c, err := LoadAppConfig("myapp", tpl)
	if err != nil {
		t.Fatal(err)
	}
	port, ok := c.Get("port")
	if !ok {
		t.Fatal("port missing")
	}
	if _, isFloat := port.(float64); !isFloat {
		t.Fatalf("first run: port is %T, want float64", port)
	}

	c2, err := LoadAppConfig("myapp", tpl)
	if err != nil {
		t.Fatal(err)
	}
	port, ok = c2.Get("port")
	if !ok {
		t.Fatal("port missing")
	}
	if _, isFloat := port.(float64); !isFloat {
		t.Fatalf("second run: port is %T, want float64", port)
	}
}

// useTempConfigDir points os.UserConfigDir at a fresh temp dir so tests
// don't touch the real user config.
func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	c := &Config{path: path, data: map[string]any{}}
	c.Set("key", "value")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify content is correct after atomic save.
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if val, ok := got.Get("key"); !ok || val != "value" {
		t.Fatalf("key = %v, %v; want value, true", val, ok)
	}

	// Verify file permissions are 0644 (mask out umask bits on Unix).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&configFileMode != configFileMode {
		t.Fatalf("permissions = %o, want at least %o", perm, configFileMode)
	}

	// Save multiple times and verify no leftover temp files.
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
