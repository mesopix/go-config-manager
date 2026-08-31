package configmanager

import (
	"path/filepath"
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

// useTempConfigDir points os.UserConfigDir at a fresh temp dir so tests
// don't touch the real user config.
func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS
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
