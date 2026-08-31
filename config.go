// Package configmanager is a tiny JSON-file-backed configuration store.
package configmanager

import (
	"encoding/json"
	"os"
)

// Config holds configuration values backed by a JSON file on disk.
type Config struct {
	path string
	data map[string]any
}

// New returns an empty Config that will be saved to path.
func New(path string) *Config {
	return &Config{path: path, data: map[string]any{}}
}

// Load reads the JSON file at path into a new Config.
// A missing file is an error; callers that want an empty config should use New.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := New(path)
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
