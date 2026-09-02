// 包 configmanager 是一个基于 JSON 文件的轻量级配置存储库。
package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadAppConfig 加载 appName 对应的配置。首次运行时从 template 创建配置文件，
// 因此除非发生 I/O 或解析错误，否则总会返回一个配置对象。
// 数值会从磁盘读回，所以类型统一为 float64。
func LoadAppConfig(appName string, template map[string]any) (*Config, error) {
	// 拼凑配置路径：<用户配置目录>/<appName>/config.json
	dir, err := os.UserConfigDir()

	// 如果拼凑路径失败
	if err != nil {
		return nil, err
	}
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
	// configDirMode 为 rwxr-xr-x：目录只需对其他人可列出的权限即可。
	const configDirMode = 0o755
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return nil, err
	}

	// 构造配置对象，这时候构造出来的配置对象 c 的 data 字段是空的
	c = &Config{path: path, data: map[string]any{}}
	// 将模板中的键值对设置到配置对象中
	for k, v := range template {
		c.Set(k, v)
	}

	// 保存配置到文件
	if err := c.Save(); err != nil {
		return nil, err
	}
	return load(path)
}

// Config 持有以磁盘 JSON 文件为后端的配置值。
type Config struct {
	path string
	data map[string]any
}

// Get 返回 key 下存储的值以及它是否存在。
// JSON 数值会被反序列化为 float64。
func (c *Config) Get(key string) (any, bool) {
	v, ok := c.data[key]
	return v, ok
}

// Set 将 value 存储到 key 下。
func (c *Config) Set(key string, value any) {
	c.data[key] = value
}

// configFileMode 为 rw-r--r--：属主可读写，其他人只读。
const configFileMode = 0o644

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
// 文件不存在视为错误。
func load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{path: path, data: map[string]any{}}
	if err := json.Unmarshal(b, &c.data); err != nil {
		return nil, err
	}
	return c, nil
}
