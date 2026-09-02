// 包 configmanager 是一个基于 JSON 文件的轻量级配置存储库。
package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configDirMode 为 rwx------：仅属主可访问配置目录。
const configDirMode = 0o700

// configFileMode 为 rw-------：仅属主可读写，防止敏感配置泄露。
const configFileMode = 0o600

// LoadAppConfig 加载 appName 对应的配置。首次运行时从 defaultJSON 创建配置文件，
// 因此除非发生 I/O 或解析错误，否则总会返回一个配置对象。
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
	c, err := load(path)

	// 如果加载成功，就直接返回配置对象
	if err == nil {
		return c, nil
	}

	// 如果不是文件不存在的错误，就返回错误（因为文件不存在不算错误，只是需要本函数后面的逻辑做初始化创建）
	if !os.IsNotExist(err) {
		return nil, err
	}

	// 代码能走到这里，说明是第一次运行
	// 首先创建配置目录
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return nil, err
	}

	// 将默认 JSON 解析为 map
	var defaults map[string]any
	if err := json.Unmarshal(defaultJSON, &defaults); err != nil {
		return nil, err
	}

	// 构造配置对象并填充默认值
	c = &Config{path: path, data: defaults, resolvedVersion: UnknownVersion}
	c.declaredVersion = extractVersion(defaults)

	// 保存配置到文件
	if err := c.Save(); err != nil {
		return nil, err
	}

	// 从磁盘重读配置文件
	return load(path)
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

// extractVersion 从 {meta: {schema_version: "..."}} 中提取版本号。
// 任何缺失或类型不匹配均返回 UnknownVersion。
func extractVersion(data map[string]any) string {
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		return UnknownVersion
	}
	v, ok := meta["schema_version"].(string)
	if !ok || v == "" {
		return UnknownVersion
	}
	return v
}
