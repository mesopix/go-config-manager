// Package configmanager is a tiny JSON-file-backed configuration store.
package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadAppConfig loads the config for appName. On first run the file is created
// from template, so a config is always returned unless an I/O or parse
// error occurs. Values are read back from disk, so numbers are float64.
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
	// configDirMode is rwxr-xr-x: dirs only need to be listable by others.
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

// Config holds configuration values backed by a JSON file on disk.
type Config struct {
	path string
	data map[string]any
}

// Get returns the value stored under key and whether it exists.
// JSON numbers unmarshal as float64.
func (c *Config) Get(key string) (any, bool) {
	v, ok := c.data[key]
	return v, ok
}

// Set stores value under key.
func (c *Config) Set(key string, value any) {
	c.data[key] = value
}

// configFileMode is rw-r--r--: owner reads/writes, others read-only.
const configFileMode = 0o644

// Save writes the current values back to the JSON file atomically.
// It writes to a temporary file first, syncs to disk, then renames
// over the target so a crash never leaves a half-written config.
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}

	// Create temp file in the same directory as the target
	// so os.Rename works (must be on the same filesystem).
	tmpFile, err := os.CreateTemp(filepath.Dir(c.path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Clean up the temp file if anything below fails.
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

	// Sync ensures data reaches stable storage before rename.
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

// load reads the JSON file at path into a new Config.
// A missing file is an error.
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
