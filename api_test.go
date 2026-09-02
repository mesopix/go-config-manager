package configmanager_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	configmanager "github.com/mesopix/go-config-manager"
)

// useTempConfigDir 将 os.UserConfigDir 指向临时目录。
// 与 config_test.go 中的同名函数逻辑相同，但因外部测试包无法访问
// 未导出符号，故在此独立定义一份。这是 Go 测试的标准做法。
func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS
}

// ---------- 从模板创建配置（迁入） ----------

func TestLoadAppConfigCreatesFromTemplate(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)
	c, err := configmanager.LoadAppConfig("myapp", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	if name, ok := c.Get("name"); !ok || name != "demo" {
		t.Fatalf("name = %v, %v", name, ok)
	}
	if port, ok := c.Get("port"); !ok || port != float64(8080) {
		t.Fatalf("port = %v, %v", port, ok)
	}

	// 第二次加载从磁盘读取，并保留之前的修改。
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := configmanager.LoadAppConfig("myapp", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}
	if port, ok := c2.Get("port"); !ok || port != float64(9090) {
		t.Fatalf("port = %v, %v", port, ok)
	}
}

// ---------- 数值类型契约（迁入） ----------

// 演示上一轮讨论的坑：模板里的 8080 在 JSON 中是数字，但首次运行也走
// "写盘→读回"，所以首末两次运行取出的类型一致，都是 float64。
func TestLoadAppConfigNumbersAreFloat64(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"port": 8080}}`)

	c, err := configmanager.LoadAppConfig("myapp", defaultJSON)
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

	c2, err := configmanager.LoadAppConfig("myapp", defaultJSON)
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

// ---------- 非法 defaultJSON（新增） ----------

func TestLoadAppConfig_invalidDefaultJSON(t *testing.T) {
	useTempConfigDir(t)

	invalidJSON := []byte(`{not valid json}`)
	_, err := configmanager.LoadAppConfig("badapp", invalidJSON)
	if err == nil {
		t.Fatal("LoadAppConfig with invalid defaultJSON: expected error, got nil")
	}
}

// ---------- 自动创建配置文件（新增） ----------

func TestLoadAppConfig_autoCreatesWhenMissing(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost", "port": 3000}}`)
	c, err := configmanager.LoadAppConfig("newapp", defaultJSON)
	if err != nil {
		t.Fatalf("first LoadAppConfig: unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("first LoadAppConfig: returned nil Config")
	}

	if host, ok := c.Get("host"); !ok || host != "localhost" {
		t.Errorf("host = %v, %v; want localhost, true", host, ok)
	}
	if port, ok := c.Get("port"); !ok || port != float64(3000) {
		t.Errorf("port = %v, %v; want 3000, true", port, ok)
	}
}

// ---------- Save → Load 往返一致性（新增） ----------

func TestLoadAppConfig_saveLoadRoundTrip(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"key": "original"}}`)
	c, err := configmanager.LoadAppConfig("roundtrip", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("key", "modified")
	c.Set("extra", float64(42))
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	c2, err := configmanager.LoadAppConfig("roundtrip", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	if val, ok := c2.Get("key"); !ok || val != "modified" {
		t.Errorf("key = %v, %v; want modified, true", val, ok)
	}
	if val, ok := c2.Get("extra"); !ok || val != float64(42) {
		t.Errorf("extra = %v, %v; want 42, true", val, ok)
	}
}

// ---------- 二次加载忽略默认值（新增） ----------

func TestLoadAppConfig_secondLoadIgnoresDefaults(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"color": "red"}}`)
	c, err := configmanager.LoadAppConfig("overwrite", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("color", "blue")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// 用不同的默认值再次加载，应读到磁盘上的 "blue" 而非新默认的 "green"。
	newDefaultJSON := []byte(`{"meta": {}, "fields": {"color": "green"}}`)
	c2, err := configmanager.LoadAppConfig("overwrite", newDefaultJSON)
	if err != nil {
		t.Fatal(err)
	}
	if val, ok := c2.Get("color"); !ok || val != "blue" {
		t.Errorf("color = %v, %v; want blue (from disk), true", val, ok)
	}
}

// ---------- Get 不存在的 key（新增） ----------

func TestLoadAppConfig_getMissingKey(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"exists": true}}`)
	c, err := configmanager.LoadAppConfig("missingkey", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost", "port": 8080}}`)
	c, err := configmanager.LoadAppConfig("schematest", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"host": {Type: configmanager.TypeString, Required: true},
		"port": {Type: configmanager.TypeFloat, Required: true},
	}

	// 从 Config 提取 data 用于 Check
	data := map[string]any{}
	for _, key := range []string{"host", "port"} {
		if v, ok := c.Get(key); ok {
			data[key] = v
		}
	}

	if got := schema.Check(data); got != configmanager.Valid {
		t.Errorf("Check = %d, want Valid", got)
	}
}

// ---------- Schema + Config 协作：Normalize 补全后写回 ----------

func TestSchema_normalizeAndSaveBack(t *testing.T) {
	useTempConfigDir(t)

	// 首次加载只有 host，缺少 port
	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost"}}`)
	c, err := configmanager.LoadAppConfig("normtest", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"host": {Type: configmanager.TypeString, Required: true},
		"port": {Type: configmanager.TypeFloat, Default: float64(3000)},
	}

	// 提取当前 data
	data := map[string]any{}
	if v, ok := c.Get("host"); ok {
		data["host"] = v
	}

	// Check 应为 MissingDefaults
	if got := schema.Check(data); got != configmanager.MissingDefaults {
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

	// 重新加载验证 port 已持久化
	c2, err := configmanager.LoadAppConfig("normtest", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}
	if port, ok := c2.Get("port"); !ok || port != float64(3000) {
		t.Errorf("port after normalize+save = %v, %v; want 3000, true", port, ok)
	}
}

// ---------- 坏文件：专用错误类型 ----------

// 已存在的配置文件损坏时，LoadAppConfig 应返回 nil 和 CorruptConfigError，
// 不提供默认值降级，且不覆盖磁盘上的坏文件。
func TestLoadAppConfig_corruptFileReturnsTypedError(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {"version": "1"}, "fields": {"color": "red"}}`)

	// 先正常创建配置文件
	if _, err := configmanager.LoadAppConfig("corruptapp", defaultJSON); err != nil {
		t.Fatal(err)
	}

	// 手动把配置文件改坏
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "corruptapp", "config.json")
	corrupted := []byte(`{invalid json`)
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}

	// 应返回错误和 nil 配置，杜绝调用方拿默认值静默继续
	c, err := configmanager.LoadAppConfig("corruptapp", defaultJSON)
	if err == nil {
		t.Fatal("LoadAppConfig with corrupt file: expected error, got nil")
	}
	if c != nil {
		t.Errorf("LoadAppConfig with corrupt file: expected nil Config, got %v", c)
	}

	// 错误可通过 errors.As 识别为 CorruptConfigError，且携带正确路径
	var corruptErr *configmanager.CorruptConfigError
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

// RepairAppConfig 是预留接口，当前阶段固定返回未实现错误。
func TestRepairAppConfig_notImplementedYet(t *testing.T) {
	useTempConfigDir(t)

	err := configmanager.RepairAppConfig("anyapp", []byte(`{"meta": {}, "fields": {}}`))
	if err == nil {
		t.Fatal("RepairAppConfig: expected error, got nil")
	}
}

// ---------- 结构体绑定：DecodeFields / SetFieldsFrom ----------

type sampleSettings struct {
	Name    string            `json:"name"`
	Port    float64           `json:"port"`
	Debug   bool              `json:"debug,omitempty"`
	Tags    []string          `json:"tags,omitempty"`
	Meta    map[string]any    `json:"meta,omitempty"`
	Optional *string         `json:"optional,omitempty"`
}

// DecodeFields 往返一致：写入 → 读回字段值相同。
func TestStructBinding_roundTrip(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"name": "demo", "port": 8080}}`)
	c, err := configmanager.LoadAppConfig("binding", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

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

	c2, err := configmanager.LoadAppConfig("binding", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}
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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"name": "only-name"}}`)
	c, err := configmanager.LoadAppConfig("partial", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

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
	useTempConfigDir(t)

	// optional 显式为 null（JSON null 解码到 *string 为 nil）——用空串更直观
	defaultJSON := []byte(`{"meta": {}, "fields": {"optional": ""}}`)
	c, err := configmanager.LoadAppConfig("explicit", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}
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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {}}`)
	c, err := configmanager.LoadAppConfig("reject", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost", "port": 8080}}`)
	c, err := configmanager.LoadAppConfig("cfgcheck", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"host": {Type: configmanager.TypeString, Required: true},
		"port": {Type: configmanager.TypeFloat, Required: true},
	}
	if got := c.Check(schema); got != configmanager.Valid {
		t.Errorf("Check = %d, want Valid", got)
	}
}

// Config.Normalize 在 Valid 状态下为 no-op，不报错。
func TestConfig_normalizeValidIsNoop(t *testing.T) {
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"key": "value"}}`)
	c, err := configmanager.LoadAppConfig("noopnorm", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"key": {Type: configmanager.TypeString, Required: true},
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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost"}}`)
	c, err := configmanager.LoadAppConfig("normfill", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"host": {Type: configmanager.TypeString, Required: true},
		"port": {Type: configmanager.TypeFloat, Default: float64(3000)},
	}
	if got := c.Check(schema); got != configmanager.MissingDefaults {
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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"host": "localhost", "extra": "junk"}}`)
	c, err := configmanager.LoadAppConfig("normextra", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"host": {Type: configmanager.TypeString, Required: true},
	}
	if got := c.Check(schema); got != configmanager.ExtraFields {
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
	useTempConfigDir(t)

	defaultJSON := []byte(`{"meta": {}, "fields": {"port": "not-a-number"}}`)
	c, err := configmanager.LoadAppConfig("norminvalid", defaultJSON)
	if err != nil {
		t.Fatal(err)
	}

	schema := configmanager.Schema{
		"port": {Type: configmanager.TypeFloat, Required: true},
	}
	if got := c.Check(schema); got != configmanager.Invalid {
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
