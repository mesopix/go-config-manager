package goconfig

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Run executes the `config` CLI over the manager's file. args are the
// arguments after the "config" command itself (e.g. os.Args[2:]):
//
//	config --edit       open the file in $VISUAL / $EDITOR
//	config list         list all settings
//	config get <key>    print one value
//	config set <k> <v>  set one value (validated; type follows the old value)
//	config del <key>    remove one key
//
// Wire it into any CLI framework as the action of your config command.
func (m *Manager) Run(args []string) error {
	return m.run(args, os.Stdout, stdinIsTerminal())
}

// run is Run with injected output and interactivity, for tests. With no
// subcommand it opens the TUI menu when interactive, else prints usage.
func (m *Manager) run(args []string, out io.Writer, interactive bool) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	edit := fs.Bool("edit", false, "edit the config file in $VISUAL/$EDITOR")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if *edit {
		return m.Edit()
	}
	rest := fs.Args()
	switch {
	case len(rest) == 0:
		if !interactive {
			fmt.Fprint(out, usage)
			return nil
		}
		return m.TUI()
	case rest[0] == "help":
		fmt.Fprint(out, usage)
		return nil
	case rest[0] == "list":
		return m.cmdList(out)
	case rest[0] == "get":
		if len(rest) != 2 {
			return errors.New("usage: config get <key>")
		}
		return m.cmdGet(out, rest[1])
	case rest[0] == "set":
		if len(rest) != 3 {
			return errors.New("usage: config set <key> <value>")
		}
		return m.cmdSet(out, rest[1], rest[2])
	case rest[0] == "del":
		if len(rest) != 2 {
			return errors.New("usage: config del <key>")
		}
		return m.cmdDel(out, rest[1])
	default:
		return fmt.Errorf("config: unknown command %q", rest[0])
	}
}

const usage = `usage: config [command]

  config              open the interactive settings menu
  config --edit       open the config file in $VISUAL / $EDITOR
  config list         list all settings
  config get <key>    print one value
  config set <k> <v>  set one value
  config del <key>    remove one key
  config help         show this help
`

func (m *Manager) cmdList(out io.Writer) error {
	doc, err := m.Load()
	if err != nil {
		return err
	}
	for _, k := range doc.Keys() {
		v, _ := doc.Get(k)
		fmt.Fprintf(out, "%s = %v\n", k, v)
	}
	return nil
}

func (m *Manager) cmdGet(out io.Writer, key string) error {
	doc, err := m.Load()
	if err != nil {
		return err
	}
	v, ok := doc.Get(key)
	if !ok {
		return fmt.Errorf("config: key %q not found", key)
	}
	if _, isTable := v.(map[string]any); isTable {
		return fmt.Errorf("config: %q is a table; get a leaf key (see: config list)", key)
	}
	fmt.Fprintf(out, "%v\n", v)
	return nil
}

func (m *Manager) cmdSet(out io.Writer, key, value string) error {
	doc, err := m.Load()
	if err != nil {
		return err
	}
	v, err := m.applySet(doc, key, value)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s = %v\n", key, v)
	return nil
}

// applySet coerces value to the old value's type, validates on a trial copy
// and persists. On success doc is kept in sync with the file; on any failure
// doc and file are left untouched. Shared by `config set` and the TUI.
func (m *Manager) applySet(doc *Doc, key, value string) (any, error) {
	old, _ := doc.Get(key)
	v, err := coerceValue(value, old)
	if err != nil {
		return nil, fmt.Errorf("config: set %s: %w", key, err)
	}
	trial, err := copyDoc(doc)
	if err != nil {
		return nil, fmt.Errorf("config: set %s: %w", key, err)
	}
	trial.Set(key, v)
	if err := m.validate(trial); err != nil {
		return nil, fmt.Errorf("config: set %s: %w", key, err)
	}
	if err := m.Save(trial); err != nil {
		return nil, err
	}
	doc.Set(key, v)
	return v, nil
}

func (m *Manager) cmdDel(out io.Writer, key string) error {
	doc, err := m.Load()
	if err != nil {
		return err
	}
	if !doc.Delete(key) {
		return fmt.Errorf("config: key %q not found", key)
	}
	if err := m.validate(doc); err != nil {
		return fmt.Errorf("config: del %s: %w", key, err)
	}
	if err := m.Save(doc); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s removed\n", key)
	return nil
}

// coerceValue converts the CLI string to the TOML type of the previous value.
// New keys are guessed in order: int64, float64, bool, then string.
func coerceValue(s string, old any) (any, error) {
	switch old.(type) {
	case int64:
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i, nil
		}
		return nil, fmt.Errorf("value %q is not an integer", s)
	case float64:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, nil
		}
		return nil, fmt.Errorf("value %q is not a float", s)
	case bool:
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		}
		return nil, fmt.Errorf("value %q is not a bool", s)
	case string:
		return s, nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b, nil
	}
	return s, nil
}

// Edit opens the config file in the user's editor ($VISUAL, $EDITOR, then
// notepad on Windows / vi elsewhere) and reloads it afterwards (parse,
// migrate, validate). The pre-edit copy is kept at Path+".bak"; a file that
// fails to load is left exactly as the user saved it.
func (m *Manager) Edit() error {
	if _, err := m.Load(); err != nil {
		return err
	}
	orig, err := os.ReadFile(m.opts.Path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.opts.Path+".bak", orig, 0o600); err != nil {
		return fmt.Errorf("config %s: backup before edit: %w", m.opts.Path, err)
	}
	editor := resolveEditor()
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], m.opts.Path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q failed: %w", editor, err)
	}
	if _, err := m.Load(); err != nil {
		return fmt.Errorf("%w\nthe edited file was left as-is; the pre-edit copy is at %s.bak",
			err, m.opts.Path)
	}
	return nil
}

func resolveEditor() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

// stdinIsTerminal reports whether stdin is an interactive terminal, so the
// TUI menu is skipped (usage printed instead) when config runs in a pipe/CI.
// ModeCharDevice via Stat is not enough: on Windows the NUL device reports
// as a char device too.
func stdinIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}
