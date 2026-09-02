// 包 configmanager 是一个基于 JSON 文件的轻量级配置存储库。
package configmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// configDirMode 为 rwx------：仅属主可访问配置目录。
const configDirMode = 0o700

// configFileMode 为 rw-------：仅属主可读写，防止敏感配置泄露。
const configFileMode = 0o600

// LoadAppConfig 加载 appName 对应的配置。首次运行时从 defaultJSON 创建配置文件，
// 因此除非无法获取用户配置目录或 defaultJSON 本身非法，否则总会返回一个配置对象。
// 已存在的配置文件无法读取或解析时，返回 nil 和 *CorruptConfigError，
// 不提供默认值降级，也不覆盖磁盘上的坏文件。
// defaultJSON 应为合法的 JSON 对象（如通过 //go:embed 嵌入的模板文件）。
// 数值会从磁盘读回，所以类型统一为 float64。
func LoadAppConfig(appName string, defaultJSON []byte) (*Config, error) {
	// 获取用户配置目录
	dir, err := os.UserConfigDir()
	// 获取失败，返回
	if err != nil {
		return nil, err
	}

	// 拼凑配置路径：<用户配置目录>/<appName>/config.json
	path := filepath.Join(dir, appName, "config.json")

	// 直接尝试加载
	loaded, err := load(path)

	// 如果加载成功，就直接返回配置对象
	if err == nil {
		return loaded, nil
	}

	// 文件存在但无法读取或解析：返回专用错误类型。
	// 不提供默认值降级，杜绝调用方拿默认值静默继续；
	// 不覆盖磁盘上的坏文件，留给 RepairAppConfig 或人工处置。
	if !os.IsNotExist(err) {
		return nil, &CorruptConfigError{Path: path, Err: err}
	}

	// 代码能走到这里，说明是第一次运行
	// 首先创建配置目录
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return nil, err
	}

	// 解析默认 JSON 并构造配置对象
	created, err := newConfigFromDefaults(path, defaultJSON)
	if err != nil {
		return nil, err
	}

	// 保存配置到文件
	if err := created.Save(); err != nil {
		return nil, err
	}

	// 从磁盘重读配置文件
	return load(path)
}

// newConfigFromDefaults 解析 defaultJSON 并构造 Config，不读写磁盘。
// defaultJSON 解析失败时返回 nil 和错误。
func newConfigFromDefaults(path string, defaultJSON []byte) (*Config, error) {
	var defaults map[string]any
	if err := json.Unmarshal(defaultJSON, &defaults); err != nil {
		return nil, err
	}
	config := &Config{path: path, data: defaults, resolvedVersion: UnknownVersion}
	config.declaredVersion = extractVersion(defaults)
	return config, nil
}

// CorruptConfigError 表示已存在的配置文件无法读取或解析。
// 调用方应用 errors.As 识别本错误，向用户报错并退出，不得静默改用默认值。
type CorruptConfigError struct {
	Path string // 配置文件的绝对路径
	Err  error  // 原始读取或解析错误
}

// Error 实现 error 接口，信息携带配置文件路径与原始错误。
func (e *CorruptConfigError) Error() string {
	return fmt.Sprintf("config file %s is corrupt or unreadable: %v", e.Path, e.Err)
}

// Unwrap 返回原始错误，支持 errors.Is/As 链式判断。
func (e *CorruptConfigError) Unwrap() error {
	return e.Err
}

// errRepairNotImplemented 是预留修复接口的占位错误。
var errRepairNotImplemented = errors.New("config repair is not implemented yet")

// RepairAppConfig 修复 appName 对应的损坏配置文件。预留接口，尚未实现；
// 未来版本将基于 defaultJSON 重建配置文件或引导用户修复。
func RepairAppConfig(appName string, defaultJSON []byte) error {
	return errRepairNotImplemented
}

// UnknownVersion 表示无法识别的数据版本号。
const UnknownVersion = "UNKNOWN"

// Config 持有以磁盘 JSON 文件为后端的配置值。
// data 存储完整的 {meta, fields} 两层结构。
type Config struct {
	path            string
	data            map[string]any
	declaredVersion string // 从 config.json meta.schema_version 读取的原始版本
	resolvedVersion string // 经 schema 校验后确定的实际版本
}

// Path 返回配置文件的绝对路径。
func (c *Config) Path() string {
	return c.path
}

// DeclaredVersion 返回 config.json 中声明的数据版本号。
// 无法识别时返回 UnknownVersion。
func (c *Config) DeclaredVersion() string {
	return c.declaredVersion
}

// ResolvedVersion 返回经 schema 校验后确定的实际数据版本号。
// 未经校验或无法识别时返回 UnknownVersion。
func (c *Config) ResolvedVersion() string {
	return c.resolvedVersion
}

// ResolveVersion 在 schema 校验通过后调用，将 resolvedVersion 设为
// schema 的 meta.version。若 schemaVersion 为空则设为 UnknownVersion。
func (c *Config) ResolveVersion(schemaVersion string) {
	if schemaVersion == "" {
		c.resolvedVersion = UnknownVersion
	} else {
		c.resolvedVersion = schemaVersion
	}
}

// fields 返回 data 中 "fields" 层的 map。
// 若不存在则返回空 map（不修改 data）。
func (c *Config) fields() map[string]any {
	if f, ok := c.data["fields"].(map[string]any); ok {
		return f
	}
	return map[string]any{}
}

// Meta 返回 data 中 "meta" 层的 map（只读访问）。
func (c *Config) Meta() map[string]any {
	if m, ok := c.data["meta"].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// Get 返回 fields 层中 key 下存储的值以及它是否存在。
// JSON 数值会被反序列化为 float64。
func (c *Config) Get(key string) (any, bool) {
	v, ok := c.fields()[key]
	return v, ok
}

// Set 将 value 存储到 fields 层的 key 下。
func (c *Config) Set(key string, value any) {
	if _, ok := c.data["fields"].(map[string]any); !ok {
		c.data["fields"] = map[string]any{}
	}
	c.data["fields"].(map[string]any)[key] = value
}

// Save 以原子方式把当前值写回 JSON 文件。
// 先写入临时文件并同步到磁盘，再重命名覆盖目标文件，
// 这样即使中途崩溃也不会留下写了一半的配置。
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}

	// 在与目标文件相同的目录下创建临时文件，
	// 保证 os.Rename 可用（必须在同一文件系统上）。
	tmpFile, err := os.CreateTemp(filepath.Dir(c.path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// 后续任何步骤失败时清理临时文件。
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(b); err != nil {
		tmpFile.Close()
		return err
	}

	// Sync 确保数据在 rename 之前落盘。
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, configFileMode); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		return err
	}

	success = true
	return nil
}

// ---------- 结构体绑定 ----------

// DecodeFields 将 fields 层按 JSON tag 解码到 target（必须为指针）。
// 经 JSON 往返实现：fields 中缺失的键不会改动 target 的对应字段，
// 因此指针字段可区分"未设置"（nil）与"显式零值"（指向零值的指针）。
func (c *Config) DecodeFields(target any) error {
	encoded, err := json.Marshal(c.fields())
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

// SetFieldsFrom 将 source 按 JSON tag 编码后整体替换 fields 层，meta 层不受影响。
// source 为 nil 或无法编码为 JSON 对象时返回错误。
func (c *Config) SetFieldsFrom(source any) error {
	if source == nil {
		return errors.New("configmanager: SetFieldsFrom source must not be nil")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("configmanager: SetFieldsFrom source must encode to a JSON object")
	}
	c.data["fields"] = fields
	return nil
}

// ---------- Check / Normalize 接入 Config ----------

// Check 用 schema 校验当前 fields 层，返回校验状态。
// 等价于在 fields() 上调用 Schema.Check，但无需手动提取 map。
func (c *Config) Check(schema Schema) CheckResult {
	return schema.Check(c.fields())
}

// Normalize 按 schema 规范化当前 fields 层：补全缺失的默认值、删除多余字段。
// Valid 状态下为 no-op（直接返回 nil）；MissingDefaults / ExtraFields /
// MissingAndExtra 状态下执行规范化并写回；Invalid 状态返回错误。
func (c *Config) Normalize(schema Schema) error {
	switch c.Check(schema) {
	case Valid:
		return nil
	case MissingDefaults, ExtraFields, MissingAndExtra:
		normalized, err := schema.Normalize(c.fields())
		if err != nil {
			return err
		}
		c.data["fields"] = normalized
		return nil
	default:
		return fmt.Errorf("cannot normalize: check result is Invalid")
	}
}

// load 读取 path 处的 JSON 文件到一个新的 Config 中。
// 文件不存在视为错误。加载后自动提取 declaredVersion。
func load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{path: path, data: map[string]any{}, resolvedVersion: UnknownVersion}
	if err := json.Unmarshal(b, &c.data); err != nil {
		return nil, err
	}
	// 提取 declaredVersion
	c.declaredVersion = extractVersion(c.data)
	return c, nil
}

// extractVersion 从 {meta: {version: "..."}} 中提取版本号。
// 任何缺失或类型不匹配均返回 UnknownVersion。
func extractVersion(data map[string]any) string {
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		return UnknownVersion
	}
	v, ok := meta["version"].(string)
	if !ok || v == "" {
		return UnknownVersion
	}
	return v
}
