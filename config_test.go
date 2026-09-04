package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestConfig 在独立临时目录中完成 Init + RegisterDefaults + Load，
// 返回全局单例；测试结束时自动 Reset，避免用例间串扰。
func newTestConfig(t *testing.T, defaultJSON []byte) *Config {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
	if err := Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := RegisterDefaults(defaultJSON); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
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

// ---------- 全局装配：Init / RegisterDefaults / Load ----------

// 未注册模板且文件不存在时，Load 应报错而不是静默创建空配置。
func TestLoad_missingFileWithoutTemplate(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	if err := Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load without template and missing file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no defaults registered") {
		t.Errorf("error = %q, want contain %q", err.Error(), "no defaults registered")
	}
}

// Load 是进程内单例：后续调用返回同一对象，并填充导出的全局 ConfigManager。
func TestLoad_returnsSingleton(t *testing.T) {
	c := newTestConfig(t, []byte(`{"meta": {}, "fields": {"key": "value"}}`))

	c2, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if c2 != c {
		t.Error("Load should return the same singleton instance")
	}
	if ConfigManager != c {
		t.Error("ConfigManager should point to the loaded singleton")
	}

	// Reset 后 ConfigManager 归零，允许重新装配
	Reset()
	if ConfigManager != nil {
		t.Error("ConfigManager should be nil after Reset")
	}
	if err := Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init after Reset: %v", err)
	}
}

// Init 重复调用报错；Reset 之后允许重新装配。
func TestInit_onlyOnce(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	if err := Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := Init(t.TempDir(), "app", "config.json"); err == nil {
		t.Fatal("second Init: expected error, got nil")
	}

	// Reset 后可重新装配
	Reset()
	if err := Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init after Reset: %v", err)
	}
}

// Init 参数校验（表驱动）：相对路径、绝对/上跳二级目录、带路径成分的文件名。
func TestInit_invalidArguments(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		label                         string
		firstDir, secondDir, fileName string
	}{
		{"relative firstDir", "relative/path", "app", "config.json"},
		{"absolute secondDir", dir, dir, "config.json"},
		{"traversal secondDir", dir, "a/../../escape", "config.json"},
		{"fileName with separator", dir, "app", "sub/config.json"},
		{"dot fileName", dir, "app", "."},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)
			if err := Init(tt.firstDir, tt.secondDir, tt.fileName); err == nil {
				t.Errorf("Init(%q, %q, %q): expected error, got nil", tt.firstDir, tt.secondDir, tt.fileName)
			}
		})
	}
}

// RegisterDefaults 只能注册一次，且模板必须是 JSON 对象。
func TestRegisterDefaults_validation(t *testing.T) {
	tests := []struct {
		label    string
		template string
		wantErr  bool
	}{
		{"valid object", `{"meta": {}, "fields": {}}`, false},
		{"syntax error", `{invalid`, true},
		{"array instead of object", `[1, 2]`, true},
		{"empty input", ``, true},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)
			err := RegisterDefaults([]byte(tt.template))
			if (err != nil) != tt.wantErr {
				t.Fatalf("RegisterDefaults: error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// 第二次注册报错
			if err := RegisterDefaults([]byte(`{}`)); err == nil {
				t.Error("second RegisterDefaults: expected error, got nil")
			}
		})
	}
}

// ---------- 目录权限 ----------

func TestInitDirPermissions(t *testing.T) {
	c := newTestConfig(t, []byte(`{"meta": {"schema_version": "1.0.0"}, "fields": {"key": "value"}}`))

	// 验证配置目录权限至少为 0700。
	info, err := os.Stat(filepath.Dir(c.Path()))
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
