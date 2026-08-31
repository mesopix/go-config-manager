// Package configmanager is a tiny JSON-file-backed configuration store.
package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds configuration values backed by a JSON file on disk.
type Config struct {
	path string
	data map[string]any
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

// Save writes the current values back to the JSON file.
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, configFileMode)
}

// configDirMode is rwxr-xr-x: dirs only need to be listable by others.
const configDirMode = 0o755

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
