package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestConfig 在独立临时目录中用全新 Manager 完成 Init + RegisterDefaults + Load，
// 返回该 Manager 与加载好的配置对象；每个用例各自持有实例，无需全局清理。
func newTestConfig(t *testing.T, defaultJSON []byte) (*Manager, *Config) {
	t.Helper()
	m := NewManager()
	if err := m.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.RegisterDefaults(defaultJSON); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}
	c, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return m, c
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

// ---------- 装配：Init / RegisterDefaults / Load ----------

// 未注册模板且文件不存在时，Load 应报错而不是静默创建空配置。
func TestLoad_missingFileWithoutTemplate(t *testing.T) {
	m := NewManager()
	if err := m.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err := m.Load()
	if err == nil {
		t.Fatal("Load without template and missing file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no defaults registered") {
		t.Errorf("error = %q, want contain %q", err.Error(), "no defaults registered")
	}
}

// Load 幂等：同一 Manager 后续调用返回同一对象；
// 指向同一目录的第二个 Manager（模拟进程重启）从磁盘加载另一实例。
func TestLoad_idempotent(t *testing.T) {
	dir := t.TempDir()
	template := []byte(`{"meta": {}, "fields": {"key": "value"}}`)

	m := NewManager()
	if err := m.Init(dir, "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.RegisterDefaults(template); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}
	c, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	c2, err := m.Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if c2 != c {
		t.Error("Load should return the same instance")
	}

	m2 := NewManager()
	if err := m2.Init(dir, "app", "config.json"); err != nil {
		t.Fatalf("Init second manager: %v", err)
	}
	if err := m2.RegisterDefaults(template); err != nil {
		t.Fatalf("RegisterDefaults second manager: %v", err)
	}
	c3, err := m2.Load()
	if err != nil {
		t.Fatalf("Load second manager: %v", err)
	}
	if c3 == c {
		t.Error("two managers should hold distinct Config instances")
	}
	if c3.Path() != c.Path() {
		t.Errorf("path = %q, want %q", c3.Path(), c.Path())
	}
	if v, ok := c3.Get("key"); !ok || v != "value" {
		t.Errorf("key = %v, %v; want value, true", v, ok)
	}
}

// Init 重复调用报错；全新 Manager 可正常装配，实例间互不干扰。
func TestInit_onlyOnce(t *testing.T) {
	m := NewManager()
	if err := m.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := m.Init(t.TempDir(), "app", "config.json"); err == nil {
		t.Fatal("second Init: expected error, got nil")
	}

	// 新 Manager 与旧实例互不影响
	m2 := NewManager()
	if err := m2.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init on fresh Manager: %v", err)
	}
}

// 懒装配 Load 成功后（cfg 非 nil）Init 必须报错，
// 覆盖 once-guard 中 m.cfg != nil 这一半。
func TestInit_afterLazyLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS

	m := NewManager()
	if err := m.RegisterDefaults([]byte(`{"meta": {}, "fields": {"lazy": true}}`)); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}
	if _, err := m.Load(); err != nil {
		t.Fatalf("lazy Load: %v", err)
	}
	if err := m.Init(t.TempDir(), "app", "config.json"); err == nil {
		t.Fatal("Init after successful lazy Load: expected error, got nil")
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
			m := NewManager()
			if err := m.Init(tt.firstDir, tt.secondDir, tt.fileName); err == nil {
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
			m := NewManager()
			err := m.RegisterDefaults([]byte(tt.template))
			if (err != nil) != tt.wantErr {
				t.Fatalf("RegisterDefaults: error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// 第二次注册报错
			if err := m.RegisterDefaults([]byte(`{}`)); err == nil {
				t.Error("second RegisterDefaults: expected error, got nil")
			}
		})
	}
}

// ---------- 目录权限 ----------

func TestInitDirPermissions(t *testing.T) {
	_, c := newTestConfig(t, []byte(`{"meta": {"version": "1.0.0"}, "fields": {"key": "value"}}`))

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
