package appconfig

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newTestManager 在独立临时目录装配一个全新 Manager（不加载），
// 并返回它与期望的配置文件路径。defaultJSON 为 nil 表示不注册模板；
// 每个用例各自持有实例，无需全局清理。
func newTestManager(t *testing.T, defaultJSON []byte) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager()
	if err := m.Init(dir, "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if defaultJSON != nil {
		if err := m.RegisterDefaults(defaultJSON); err != nil {
			t.Fatalf("RegisterDefaults: %v", err)
		}
	}
	return m, filepath.Join(dir, "app", "config.json")
}

// ---------- editConfig：文件缺失时先按模板创建 ----------

func TestEditConfig_createsMissingFileThenEdits(t *testing.T) {
	m, wantPath := newTestManager(t, []byte(`{"meta": {}, "fields": {"port": 3000}}`))

	var openedPath, openedContent string
	// 假编辑器：记录打开时的内容，模拟用户把 port 改成 4000 后保存退出
	m.editor = func(path string) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		openedPath, openedContent = path, string(b)
		return os.WriteFile(path, []byte(`{"meta": {}, "fields": {"port": 4000}}`), 0o600)
	}

	if err := m.editConfig(); err != nil {
		t.Fatalf("editConfig: unexpected error: %v", err)
	}

	// 假编辑器打开的路径应为装配出的配置文件路径
	if openedPath != wantPath {
		t.Errorf("editor opened %q, want %q", openedPath, wantPath)
	}
	// 打开时文件已按模板创建
	if !strings.Contains(openedContent, `"port": 3000`) {
		t.Errorf("created file content = %q, want contains %q", openedContent, `"port": 3000`)
	}
	// 用户的编辑（假编辑器写入）已落盘
	onDisk, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), `"port": 4000`) {
		t.Errorf("file after edit = %q, want contains %q", onDisk, `"port": 4000`)
	}
}

// ---------- editConfig：未注册模板且文件缺失 ----------

func TestEditConfig_missingFileWithoutTemplate(t *testing.T) {
	m, _ := newTestManager(t, nil)

	if err := m.editConfig(); err == nil {
		t.Fatal("editConfig without template and missing file: expected error, got nil")
	}
}

// ---------- editConfig：损坏文件仍打开原始坏文件 ----------

// 已损坏的配置不被覆盖也不报 CorruptConfigError，
// 而是把原始坏内容交给"编辑器"（用户），修复结果落盘。
func TestEditConfig_corruptFileStillOpened(t *testing.T) {
	m, path := newTestManager(t, []byte(`{"meta": {}, "fields": {"color": "red"}}`))

	// 手工放置一个损坏的配置文件
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupted := []byte(`{invalid json`)
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}

	var openedContent string
	// 假编辑器：记录打开时的坏内容，模拟用户手工修复后保存
	m.editor = func(p string) error {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		openedContent = string(b)
		return os.WriteFile(p, []byte(`{"meta": {}, "fields": {"color": "fixed"}}`), 0o600)
	}

	// 损坏是预期情况：不返回错误，直接交给编辑器处置
	if err := m.editConfig(); err != nil {
		t.Fatalf("editConfig on corrupt file: unexpected error: %v", err)
	}
	if openedContent != string(corrupted) {
		t.Errorf("editor opened content = %q, want raw corrupt content %q", openedContent, corrupted)
	}
	fixed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fixed), "fixed") {
		t.Errorf("file after manual fix = %q", fixed)
	}
}

// ---------- editConfig：编辑器退出后的 JSON 校验 ----------

func TestEditConfig_validationAfterEdit(t *testing.T) {
	m, path := newTestManager(t, []byte(`{"meta": {}, "fields": {}}`))

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"meta": {}, "fields": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		label       string
		written     string // 假编辑器保存的内容
		wantErr     bool
		wantContain string
	}{
		{"valid object", `{"meta": {}, "fields": {"ok": true}}`, false, ""},
		{"syntax error", `{invalid`, true, "not a valid JSON object"},
		{"array instead of object", `[1, 2, 3]`, true, "not a valid JSON object"},
		{"bare string", `"hello"`, true, "not a valid JSON object"},
		{"null after edit", `null`, true, "not a valid JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			m.editor = func(p string) error {
				return os.WriteFile(p, []byte(tt.written), 0o600)
			}
			err := m.editConfig()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("error = %q, want contain %q", err.Error(), tt.wantContain)
			}
			// 非法内容不被库覆盖，留在磁盘上等用户再修
			onDisk, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(onDisk) != tt.written {
				t.Errorf("invalid content was modified: got %q", onDisk)
			}
		})
	}
}

// ---------- editConfig：编辑器启动失败 ----------

// 编辑器启动失败（如 $EDITOR 指向不存在的程序）时错误原样上抛，
// 配置文件不被改动。
func TestEditConfig_editorLaunchFails(t *testing.T) {
	m, path := newTestManager(t, []byte(`{"meta": {}, "fields": {"key": "value"}}`))

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"meta": {}, "fields": {"key": "value"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("exec: fake-editor not found")
	m.editor = func(path string) error { return wantErr }

	if err := m.editConfig(); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

// ---------- HandleCLI → editConfig 的路由 ----------

// HandleCLI 应把 "config --edit" 分发到 editConfig 并透传其结果。
func TestHandleCLI_editRoute(t *testing.T) {
	m, _ := newTestManager(t, []byte(`{"meta": {}, "fields": {"routed": true}}`))
	calls := 0
	m.editor = func(path string) error {
		calls++
		return nil // 不改文件，只验证路由
	}

	shouldClose, err := m.HandleCLI([]string{"config", "--edit"})
	if !shouldClose {
		t.Fatal("shouldClose = false, want true")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("editor called %d times, want 1", calls)
	}
}

// ---------- 编辑器选择优先级 ----------

func TestResolveEditorCommand(t *testing.T) {
	tests := []struct {
		label       string
		visual      string
		editor      string
		wantProgram string
		wantArgs    []string
	}{
		{"visual wins", "code -w", "vim", "code", []string{"-w"}},
		{"editor fallback", "", "vim -f", "vim", []string{"-f"}},
		{"whitespace-only falls through", "   ", "nano", "nano", nil},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			program, args := resolveEditorCommand()
			if program != tt.wantProgram {
				t.Errorf("program = %q, want %q", program, tt.wantProgram)
			}
			if strings.Join(args, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Errorf("args = %q, want %q", args, tt.wantArgs)
			}
		})
	}

	// 两个环境变量都为空时回退平台默认
	t.Run("platform default", func(t *testing.T) {
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "")
		program, _ := resolveEditorCommand()
		var want string
		switch runtime.GOOS {
		case "windows":
			want = "notepad"
		case "darwin":
			want = "open"
		default:
			want = "xdg-open"
		}
		if program != want {
			t.Errorf("program = %q, want %q", program, want)
		}
	})
}
