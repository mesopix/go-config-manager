package appconfig_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/mesopix/go-config-manager"
)

// useTempConfigDir 将 os.UserConfigDir 指向临时目录并返回该目录。
// 仅供测试"未 Init 懒装配"路径使用；显式 Init 的测试直接用 t.TempDir()。
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS
	return dir
}

// newManager 在 dir 下装配一个全新 Manager（Init → 可选 RegisterDefaults），
// 之后由测试自行调用 Load。defaultJSON 为空串表示不注册模板；
// 模拟进程重启时对同一 dir 再调一次即可。
func newManager(t *testing.T, dir, defaultJSON string) *appconfig.Manager {
	t.Helper()
	m := appconfig.NewManager()
	if err := m.Init(dir, "app", "config.json"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if defaultJSON != "" {
		if err := m.RegisterDefaults([]byte(defaultJSON)); err != nil {
			t.Fatalf("RegisterDefaults: %v", err)
		}
	}
	return m
}

// loadManager 完成加载并断言成功。
func loadManager(t *testing.T, m *appconfig.Manager) *appconfig.Config {
	t.Helper()
	c, err := m.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

// ---------- 首运创建与加载 ----------

func TestLoadCreatesFromTemplate(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)

	c := loadManager(t, m)
	if name, ok := c.Get("name"); !ok || name != "demo" {
		t.Fatalf("name = %v, %v; want demo, true", name, ok)
	}
	if port, ok := c.Get("port"); !ok || port != float64(8080) {
		t.Fatalf("port = %v, %v; want 8080, true", port, ok)
	}

	// 第二个 Manager（模拟重启）从磁盘读取，并保留之前的修改。
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	m2 := newManager(t, dir, `{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)
	c2 := loadManager(t, m2)
	if port, ok := c2.Get("port"); !ok || port != float64(9090) {
		t.Fatalf("port = %v, %v; want 9090, true", port, ok)
	}
}

// ---------- 数值类型契约 ----------

// 模板里的 8080 在 JSON 中是数字，但首次运行也走"写盘→读回"，
// 所以首末两次运行取出的类型一致，都是 float64。
func TestLoadNumbersAreFloat64(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {}, "fields": {"port": 8080}}`)

	c := loadManager(t, m)
	port, ok := c.Get("port")
	if !ok {
		t.Fatal("port missing")
	}
	if _, isFloat := port.(float64); !isFloat {
		t.Fatalf("first run: port is %T, want float64", port)
	}

	// 模拟重启：新 Manager 重新装配加载
	m2 := newManager(t, dir, `{"meta": {}, "fields": {"port": 8080}}`)
	c2 := loadManager(t, m2)
	port, ok = c2.Get("port")
	if !ok {
		t.Fatal("port missing")
	}
	if _, isFloat := port.(float64); !isFloat {
		t.Fatalf("second run: port is %T, want float64", port)
	}
}

// ---------- 未注册模板且文件不存在 ----------

func TestLoad_missingFileWithoutTemplate(t *testing.T) {
	m := newManager(t, t.TempDir(), "")

	_, err := m.Load()
	if err == nil {
		t.Fatal("Load without template and missing file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no defaults registered") {
		t.Errorf("error = %q, want contain %q", err.Error(), "no defaults registered")
	}
}

// ---------- 未 Init：按全缺省值懒装配 ----------

// 不调用 Init 时，路径 = os.UserConfigDir() + 可执行文件名 + config.json。
func TestLoad_lazyAssemblyDefaults(t *testing.T) {
	dir := useTempConfigDir(t)
	m := appconfig.NewManager()
	if err := m.RegisterDefaults([]byte(`{"meta": {}, "fields": {"lazy": true}}`)); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}

	c := loadManager(t, m)
	if !strings.HasPrefix(c.Path(), dir) {
		t.Errorf("path = %q, want under %q", c.Path(), dir)
	}
	if filepath.Base(c.Path()) != "config.json" {
		t.Errorf("file name = %q, want config.json", filepath.Base(c.Path()))
	}
	if v, ok := c.Get("lazy"); !ok || v != true {
		t.Errorf("lazy = %v, %v; want true, true", v, ok)
	}
}

// ---------- Init 装配约束 ----------

// Init 只能成功调用一次；新 Manager 与旧实例互不影响。
func TestInit_onlyOnce(t *testing.T) {
	m := appconfig.NewManager()
	if err := m.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := m.Init(t.TempDir(), "app", "config.json"); err == nil {
		t.Fatal("second Init: expected error, got nil")
	}

	m2 := appconfig.NewManager()
	if err := m2.Init(t.TempDir(), "app", "config.json"); err != nil {
		t.Fatalf("Init on fresh Manager: %v", err)
	}
}

// RegisterDefaults 只能注册一次。
func TestRegisterDefaults_onlyOnce(t *testing.T) {
	m := appconfig.NewManager()
	if err := m.RegisterDefaults([]byte(`{"meta": {}, "fields": {}}`)); err != nil {
		t.Fatalf("first RegisterDefaults: %v", err)
	}
	if err := m.RegisterDefaults([]byte(`{"meta": {}, "fields": {}}`)); err == nil {
		t.Fatal("second RegisterDefaults: expected error, got nil")
	}
}

// RegisterDefaults 立即校验模板：必须是合法的 JSON 对象。
func TestRegisterDefaults_invalidTemplate(t *testing.T) {
	m := appconfig.NewManager()
	if err := m.RegisterDefaults([]byte(`{not valid json}`)); err == nil {
		t.Error("RegisterDefaults with invalid JSON: expected error, got nil")
	}
	// 注册失败后仍可重新注册合法模板（失败不占用"仅一次"名额）
	if err := m.RegisterDefaults([]byte(`{"meta": {}, "fields": {}}`)); err != nil {
		t.Errorf("RegisterDefaults after failed attempt: %v", err)
	}
}

// ---------- Save → Load 往返一致性 ----------

func TestLoad_saveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {}, "fields": {"key": "original"}}`)
	c := loadManager(t, m)

	c.Set("key", "modified")
	c.Set("extra", float64(42))
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	m2 := newManager(t, dir, `{"meta": {}, "fields": {"key": "original"}}`)
	c2 := loadManager(t, m2)

	if val, ok := c2.Get("key"); !ok || val != "modified" {
		t.Errorf("key = %v, %v; want modified, true", val, ok)
	}
	if val, ok := c2.Get("extra"); !ok || val != float64(42) {
		t.Errorf("extra = %v, %v; want 42, true", val, ok)
	}
}

// ---------- 二次加载忽略默认值 ----------

func TestLoad_secondLoadIgnoresDefaults(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {}, "fields": {"color": "red"}}`)
	c := loadManager(t, m)

	c.Set("color", "blue")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// 用不同的默认值重新装配加载，应读到磁盘上的 "blue" 而非新默认的 "green"。
	m2 := newManager(t, dir, `{"meta": {}, "fields": {"color": "green"}}`)
	c2 := loadManager(t, m2)
	if val, ok := c2.Get("color"); !ok || val != "blue" {
		t.Errorf("color = %v, %v; want blue (from disk), true", val, ok)
	}
}

// ---------- Get 不存在的 key ----------

func TestLoad_getMissingKey(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"exists": true}}`)
	c := loadManager(t, m)

	val, ok := c.Get("nonexistent")
	if ok {
		t.Errorf("Get(nonexistent): ok = true; want false")
	}
	if val != nil {
		t.Errorf("Get(nonexistent): value = %v; want nil", val)
	}
}

// ---------- Schema + Config 协作：Check 端到端 ----------

func TestSchema_checkWithConfig(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"host": "localhost", "port": 8080}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"host": {Type: appconfig.TypeString, Required: true},
		"port": {Type: appconfig.TypeFloat, Required: true},
	}

	// 从 Config 提取 data 用于 Check
	data := map[string]any{}
	for _, key := range []string{"host", "port"} {
		if v, ok := c.Get(key); ok {
			data[key] = v
		}
	}

	if got := schema.Check(data); got != appconfig.Valid {
		t.Errorf("Check = %d, want Valid", got)
	}
}

// ---------- Schema + Config 协作：Normalize 补全后写回 ----------

func TestSchema_normalizeAndSaveBack(t *testing.T) {
	dir := t.TempDir()
	// 首次加载只有 host，缺少 port
	m := newManager(t, dir, `{"meta": {}, "fields": {"host": "localhost"}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"host": {Type: appconfig.TypeString, Required: true},
		"port": {Type: appconfig.TypeFloat, Default: float64(3000)},
	}

	// 提取当前 data
	data := map[string]any{}
	if v, ok := c.Get("host"); ok {
		data["host"] = v
	}

	// Check 应为 MissingDefaults
	if got := schema.Check(data); got != appconfig.MissingDefaults {
		t.Fatalf("Check = %d, want MissingDefaults", got)
	}

	// Normalize 补全
	normalized, err := schema.Normalize(data)
	if err != nil {
		t.Fatalf("Normalize: unexpected error: %v", err)
	}

	// 将补全后的值写回 Config 并保存
	for k, v := range normalized {
		c.Set(k, v)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// 重新装配加载验证 port 已持久化
	m2 := newManager(t, dir, `{"meta": {}, "fields": {"host": "localhost"}}`)
	c2 := loadManager(t, m2)
	if port, ok := c2.Get("port"); !ok || port != float64(3000) {
		t.Errorf("port after normalize+save = %v, %v; want 3000, true", port, ok)
	}
}

// ---------- 坏文件：专用错误类型 ----------

// 已存在的配置文件损坏时，Load 应返回 nil 和 CorruptConfigError，
// 不提供默认值降级，且不覆盖磁盘上的坏文件。
func TestLoad_corruptFileReturnsTypedError(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {"version": "1"}, "fields": {"color": "red"}}`)
	c := loadManager(t, m)

	// 手动把配置文件改坏
	path := c.Path()
	corrupted := []byte(`{invalid json`)
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}

	// 新 Manager（模拟重启）加载：应返回错误和 nil 配置，杜绝调用方拿默认值静默继续
	m2 := newManager(t, dir, `{"meta": {"version": "1"}, "fields": {"color": "red"}}`)
	_, err := m2.Load()
	if err == nil {
		t.Fatal("Load with corrupt file: expected error, got nil")
	}

	// 错误可通过 errors.As 识别为 CorruptConfigError，且携带正确路径
	var corruptErr *appconfig.CorruptConfigError
	if !errors.As(err, &corruptErr) {
		t.Fatalf("error type = %T, want *CorruptConfigError", err)
	}
	if corruptErr.Path != path {
		t.Errorf("Path = %q, want %q", corruptErr.Path, path)
	}

	// 磁盘上的坏文件不能被覆盖
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, corrupted) {
		t.Errorf("corrupt file was modified: got %q", onDisk)
	}
}

// ---------- 修复接口：预留未实现 ----------

// Repair 是预留接口，当前阶段固定返回未实现错误。
func TestRepair_notImplementedYet(t *testing.T) {
	if err := appconfig.NewManager().Repair(); err == nil {
		t.Fatal("Repair: expected error, got nil")
	}
}

// ---------- 结构体绑定：DecodeFields / SetFieldsFrom ----------

type sampleSettings struct {
	Name     string         `json:"name"`
	Port     float64        `json:"port"`
	Debug    bool           `json:"debug,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
	Optional *string        `json:"optional,omitempty"`
}

// DecodeFields 往返一致：写入 → 读回字段值相同。
func TestStructBinding_roundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, `{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)
	c := loadManager(t, m)

	in := sampleSettings{
		Name:  "changed",
		Port:  9090,
		Debug: true,
		Tags:  []string{"a", "b"},
		Meta:  map[string]any{"k": "v"},
	}
	if err := c.SetFieldsFrom(in); err != nil {
		t.Fatalf("SetFieldsFrom: %v", err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	m2 := newManager(t, dir, `{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)
	c2 := loadManager(t, m2)
	var out sampleSettings
	if err := c2.DecodeFields(&out); err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if out.Name != in.Name || out.Port != in.Port || out.Debug != in.Debug {
		t.Errorf("scalar mismatch: got %+v", out)
	}
	if len(out.Tags) != 2 || out.Tags[0] != "a" || out.Tags[1] != "b" {
		t.Errorf("tags = %v", out.Tags)
	}
	if v, ok := out.Meta["k"].(string); !ok || v != "v" {
		t.Errorf("meta = %v", out.Meta)
	}
}

// DecodeFields 对 fields 中缺失的键保持 target 零值；指针字段为 nil 表示"未设置"。
func TestStructBinding_missingKeysAndNilPointer(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"name": "only-name"}}`)
	c := loadManager(t, m)

	var out sampleSettings
	if err := c.DecodeFields(&out); err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if out.Name != "only-name" {
		t.Errorf("name = %q, want only-name", out.Name)
	}
	if out.Port != 0 {
		t.Errorf("port = %v, want 0 (zero value)", out.Port)
	}
	if out.Optional != nil {
		t.Errorf("optional = %v, want nil (missing key)", out.Optional)
	}
}

// DecodeFields 能区分"显式 null/零值"与"缺失"：指针字段指向零值而非 nil。
func TestStructBinding_explicitZeroVsMissing(t *testing.T) {
	// optional 显式为 null（JSON null 解码到 *string 为 nil）——用空串更直观
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"optional": ""}}`)
	c := loadManager(t, m)

	var out sampleSettings
	if err := c.DecodeFields(&out); err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if out.Optional == nil {
		t.Fatal("optional should be non-nil when explicitly set to empty string")
	}
	if *out.Optional != "" {
		t.Errorf("*optional = %q, want empty string", *out.Optional)
	}
}

// SetFieldsFrom 拒绝 nil 源和非对象编码。
func TestStructBinding_setFieldsFromRejectsInvalid(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {}}`)
	c := loadManager(t, m)

	if err := c.SetFieldsFrom(nil); err == nil {
		t.Error("SetFieldsFrom(nil): expected error, got nil")
	}
	// 切片编码为 JSON 数组，不是对象
	if err := c.SetFieldsFrom([]int{1, 2}); err == nil {
		t.Error("SetFieldsFrom(slice): expected error, got nil")
	}
}

// ---------- Config.Check / Config.Normalize 接入 ----------

// Config.Check 直接作用于 fields 层，无需手动提取 map。
func TestConfig_checkDirectlyOnFields(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"host": "localhost", "port": 8080}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"host": {Type: appconfig.TypeString, Required: true},
		"port": {Type: appconfig.TypeFloat, Required: true},
	}
	if got := c.Check(schema); got != appconfig.Valid {
		t.Errorf("Check = %d, want Valid", got)
	}
}

// Config.Normalize 在 Valid 状态下为 no-op，不报错。
func TestConfig_normalizeValidIsNoop(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"key": "value"}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"key": {Type: appconfig.TypeString, Required: true},
	}
	if err := c.Normalize(schema); err != nil {
		t.Fatalf("Normalize on Valid data: unexpected error: %v", err)
	}
	if val, ok := c.Get("key"); !ok || val != "value" {
		t.Errorf("key after noop normalize = %v, %v; want value, true", val, ok)
	}
}

// Config.Normalize 补全缺失默认值并写回 fields 层。
func TestConfig_normalizeFillsDefaultsAndWritesBack(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"host": "localhost"}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"host": {Type: appconfig.TypeString, Required: true},
		"port": {Type: appconfig.TypeFloat, Default: float64(3000)},
	}
	if got := c.Check(schema); got != appconfig.MissingDefaults {
		t.Fatalf("before normalize: Check = %d, want MissingDefaults", got)
	}
	if err := c.Normalize(schema); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if port, ok := c.Get("port"); !ok || port != float64(3000) {
		t.Errorf("port after normalize = %v, %v; want 3000, true", port, ok)
	}
}

// Config.Normalize 删除多余字段。
func TestConfig_normalizeRemovesExtraFields(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"host": "localhost", "extra": "junk"}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"host": {Type: appconfig.TypeString, Required: true},
	}
	if got := c.Check(schema); got != appconfig.ExtraFields {
		t.Fatalf("before normalize: Check = %d, want ExtraFields", got)
	}
	if err := c.Normalize(schema); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, ok := c.Get("extra"); ok {
		t.Error("extra field should be removed after normalize")
	}
	if host, ok := c.Get("host"); !ok || host != "localhost" {
		t.Errorf("host = %v, %v; want localhost, true", host, ok)
	}
}

// Config.Normalize 在 Invalid 状态下返回错误，不修改 fields。
func TestConfig_normalizeInvalidReturnsError(t *testing.T) {
	m := newManager(t, t.TempDir(), `{"meta": {}, "fields": {"port": "not-a-number"}}`)
	c := loadManager(t, m)

	schema := appconfig.Schema{
		"port": {Type: appconfig.TypeFloat, Required: true},
	}
	if got := c.Check(schema); got != appconfig.Invalid {
		t.Fatalf("before normalize: Check = %d, want Invalid", got)
	}
	if err := c.Normalize(schema); err == nil {
		t.Fatal("Normalize on Invalid data: expected error, got nil")
	}
	// fields 未被改动
	if val, ok := c.Get("port"); !ok || val != "not-a-number" {
		t.Errorf("port after failed normalize = %v, %v; want not-a-number, true", val, ok)
	}
}

// ---------- CLI 接管：HandleCLI 分发 ----------

// 不是以 config 开头的参数不接管，也不产生错误。
func TestHandleCLI_ignoresNonConfigArgs(t *testing.T) {
	m := appconfig.NewManager()
	for _, args := range [][]string{
		{},                        // 客户端无任何参数
		{"serve"},                 // 正常业务参数
		{"--config", "x"},         // 恰好包含 config 字样的其他参数
		{"Config"},                // 大小写敏感，不接管
		{"config.json", "--edit"}, // 子命令必须完整等于 config
	} {
		shouldClose, err := m.HandleCLI(args)
		if shouldClose {
			t.Errorf("HandleCLI(%q): shouldClose = true, want false", args)
		}
		if err != nil {
			t.Errorf("HandleCLI(%q): unexpected error: %v", args, err)
		}
	}
}

// config 子命令分发：错误分支（表驱动）。
// --edit 的成功路径不在此外部测试中执行：编辑器启动函数是 Manager 上未导出
// 的字段，外部包无法注入，真跑会启动真实编辑器；该路径由内部包 cli_test.go
// 用假编辑器覆盖。
func TestHandleCLI_dispatch(t *testing.T) {
	// 用法错误分支不触碰任何装配状态，裸 Manager 即可触发
	m := appconfig.NewManager()
	tests := []struct {
		label       string
		args        []string
		wantErr     bool
		wantContain string // 错误信息必须包含的内容
	}{
		{"bare config without subcommand", []string{"config"}, true, "usage"},
		{"unknown subcommand", []string{"config", "rebuild"}, true, "usage"},
		{"--help is not implemented yet", []string{"config", "--help"}, true, "usage"},
		{"edit with extra args", []string{"config", "--edit", "extra"}, true, "usage"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			shouldClose, err := m.HandleCLI(tt.args)
			if !shouldClose {
				t.Fatalf("shouldClose = false, want true")
			}
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
		})
	}
}
