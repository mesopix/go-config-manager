// Package goconfig provides reusable configuration management for TUI tools:
// embedded templates with lazy first-run creation, schema validation,
// chained version migrations (v1->v2->v3) and editing (CLI, $EDITOR, TUI).
package goconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// versionField is the TOML key holding the schema version.
const versionField = "version"

// Doc is a parsed configuration document addressed by dot paths ("server.port").
type Doc struct {
	root map[string]any
}

// Get returns the value at a dot path, if present.
func (d *Doc) Get(path string) (any, bool) {
	cur := any(d.root)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// Set stores v at a dot path, creating intermediate tables as needed.
// A non-table value in the middle of the path is replaced by a table.
func (d *Doc) Set(path string, v any) {
	parts := strings.Split(path, ".")
	m := d.root
	for _, part := range parts[:len(parts)-1] {
		next, ok := m[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[part] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}

// Delete removes the key at a dot path and reports whether it existed.
func (d *Doc) Delete(path string) bool {
	parts := strings.Split(path, ".")
	m := d.root
	for _, part := range parts[:len(parts)-1] {
		next, ok := m[part].(map[string]any)
		if !ok {
			return false
		}
		m = next
	}
	last := parts[len(parts)-1]
	if _, ok := m[last]; !ok {
		return false
	}
	delete(m, last)
	return true
}

// Version returns the schema version stored in the top-level "version" field.
func (d *Doc) Version() (int, bool) {
	v, ok := d.Get(versionField)
	if !ok {
		return 0, false
	}
	i, ok := v.(int64)
	if !ok {
		return 0, false
	}
	return int(i), true
}

// Keys returns all leaf keys as sorted dot paths. Empty tables are listed as
// keys themselves; array elements are not addressable.
func (d *Doc) Keys() []string {
	var keys []string
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for k, v := range m {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if sub, ok := v.(map[string]any); ok {
				if len(sub) == 0 {
					keys = append(keys, path)
					continue
				}
				walk(path, sub)
				continue
			}
			keys = append(keys, path)
		}
	}
	walk("", d.root)
	sort.Strings(keys)
	return keys
}

// Migration upgrades a document from schema version From to From+1. Keys the
// function does not touch are preserved as-is.
type Migration struct {
	From    int
	Migrate func(*Doc) error
}

// Options configures a Manager.
type Options struct {
	// Path is the config file location. Parent directories are created on
	// first write.
	Path string

	// Template is the TOML content written when Path does not exist yet.
	// Embed it in the consumer binary with go:embed and pass string(tmpl).
	Template string

	// Version is the current schema version of Template. 0 disables schema
	// versioning and migrations entirely. When set, the loaded document must
	// carry an integer "version" field; older documents are advanced through
	// Migrations to Version and persisted (the previous file is kept at
	// Path+".bak"). Documents newer than Version are rejected.
	Version int

	// Migrations holds one Migration per schema step: {From: 1}, {From: 2},
	// ... The manager bumps the "version" field itself after each step.
	Migrations []Migration

	// Validate, if set, runs after every successful parse (load, migration,
	// --edit) and before persisting a CLI set/del. Returning an error
	// rejects the document.
	Validate func(*Doc) error
}

// Manager loads and saves one configuration file.
type Manager struct{ opts Options }

// New returns a Manager for the given options.
func New(opts Options) *Manager { return &Manager{opts: opts} }

// Load returns the config document, creating the file from Template on first
// run. It fails if the file exists but is not valid TOML or fails Validate.
// When Options.Version is set, older documents are migrated to it (and
// persisted, with the previous file kept at Path+".bak") before validation.
func (m *Manager) Load() (*Doc, error) {
	raw, err := os.ReadFile(m.opts.Path)
	hadFile := true
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := m.writeTemplate(); err != nil {
			return nil, err
		}
		raw = []byte(m.opts.Template)
		hadFile = false
	case err != nil:
		return nil, err
	}
	doc, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", m.opts.Path, err)
	}
	migrated, err := m.applyMigrations(doc, raw, hadFile)
	if err != nil {
		return nil, err
	}
	if !migrated {
		if err := m.validate(doc); err != nil {
			return nil, fmt.Errorf("validate %s: %w", m.opts.Path, err)
		}
	}
	return doc, nil
}

// validate runs the Options.Validate hook when set.
func (m *Manager) validate(doc *Doc) error {
	if m.opts.Validate == nil {
		return nil
	}
	return m.opts.Validate(doc)
}

// applyMigrations advances doc to Options.Version through the migration
// chain, validating and persisting the result (backing up the previous file).
// It reports whether a migration ran; version checks still apply even when it
// returns false with a nil error.
func (m *Manager) applyMigrations(doc *Doc, orig []byte, hadFile bool) (bool, error) {
	if m.opts.Version <= 0 {
		return false, nil
	}
	v, ok := doc.Version()
	if !ok {
		return false, fmt.Errorf("config %s: no integer %q field (set Options.Version = 0 to disable schema versioning)",
			m.opts.Path, versionField)
	}
	if v > m.opts.Version {
		return false, fmt.Errorf("config %s: schema version %d is newer than supported version %d; upgrade this tool",
			m.opts.Path, v, m.opts.Version)
	}
	if v == m.opts.Version {
		return false, nil
	}
	for v < m.opts.Version {
		var step *Migration
		for i := range m.opts.Migrations {
			if m.opts.Migrations[i].From == v {
				step = &m.opts.Migrations[i]
				break
			}
		}
		if step == nil {
			return false, fmt.Errorf("config %s: no migration registered from schema version %d",
				m.opts.Path, v)
		}
		if step.Migrate != nil {
			if err := step.Migrate(doc); err != nil {
				return false, fmt.Errorf("config %s: migrate v%d->v%d: %w", m.opts.Path, v, v+1, err)
			}
		}
		v++
		doc.Set(versionField, int64(v))
	}
	// Validate before persisting: a rejected migration must leave the file
	// at its old version, untouched.
	if err := m.validate(doc); err != nil {
		return false, fmt.Errorf("config %s: invalid after migration: %w", m.opts.Path, err)
	}
	if hadFile {
		if err := os.WriteFile(m.opts.Path+".bak", orig, 0o600); err != nil {
			return false, fmt.Errorf("config %s: backup before migration: %w", m.opts.Path, err)
		}
	}
	if err := m.Save(doc); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) writeTemplate() error {
	if err := os.MkdirAll(filepath.Dir(m.opts.Path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeFile(m.opts.Path, []byte(m.opts.Template)); err != nil {
		return fmt.Errorf("write default config %s: %w", m.opts.Path, err)
	}
	return nil
}

// Save atomically writes doc to the config file (temp file + rename).
func (m *Manager) Save(doc *Doc) error {
	data, err := doc.bytes()
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.opts.Path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeFile(m.opts.Path, data); err != nil {
		return fmt.Errorf("write config %s: %w", m.opts.Path, err)
	}
	return nil
}

// bytes encodes the document to TOML.
func (d *Doc) bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(d.root); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// copyDoc deep-copies doc via a TOML round-trip.
func copyDoc(doc *Doc) (*Doc, error) {
	data, err := doc.bytes()
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes TOML bytes into a Doc.
func Parse(data []byte) (*Doc, error) {
	root := map[string]any{}
	if _, err := toml.Decode(string(data), &root); err != nil {
		return nil, err
	}
	return &Doc{root: root}, nil
}

// writeFile writes data to path atomically: temp file, then rename over path.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
