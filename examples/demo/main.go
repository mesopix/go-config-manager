// Command demo shows how a TUI tool embeds goconfig: lazy template creation,
// v1->v3 chained migrations, validation, and the `config` subcommands.
//
// The config path defaults to <UserConfigDir>/demo/config.toml; set
// DEMO_CONFIG to place it elsewhere (used by the walkthrough below).
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anonymous/goconfig"
)

//go:embed template.toml
var templateTOML string

func configPath() string {
	if p := os.Getenv("DEMO_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
	return filepath.Join(dir, "demo", "config.toml")
}

func newManager(path string) *goconfig.Manager {
	return goconfig.New(goconfig.Options{
		Path:     path,
		Template: templateTOML,
		Version:  3,
		Migrations: []goconfig.Migration{
			{From: 1, Migrate: func(d *goconfig.Doc) error {
				d.Set("server.host", "localhost") // v2 added the host field
				return nil
			}},
			{From: 2, Migrate: func(d *goconfig.Doc) error {
				d.Delete("legacy_name") // v3 dropped it and added log levels
				d.Set("log.level", "info")
				return nil
			}},
		},
		Validate: func(d *goconfig.Doc) error {
			p, ok := d.Get("server.port")
			if !ok {
				return nil
			}
			port, ok := p.(int64)
			if !ok || port < 1 || port > 65535 {
				return fmt.Errorf("server.port must be an integer in 1..65535, got %v", p)
			}
			return nil
		},
	})
}

func main() {
	m := newManager(configPath())

	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := m.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	doc, err := m.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
	fmt.Println("demo running with:")
	for _, k := range doc.Keys() {
		v, _ := doc.Get(k)
		fmt.Printf("  %s = %v\n", k, v)
	}
}
