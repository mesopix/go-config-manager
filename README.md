# go-config-manager

Per-app configuration stored as JSON in the user config directory. Zero dependencies.

```go
tpl := map[string]any{"port": 8080, "debug": true}
c, err := configmanager.LoadAppConfig("myapp", tpl) // always returns a config
if err != nil { ... }

port, _ := c.Get("port")
c.Set("port", 9090)
if err := c.Save(); err != nil { ... }
```

The file lives at `<user config dir>/myapp/config.json`; on first run it is created from the template.

Note: JSON numbers come back as `float64` from `Get`.

## Demo

```sh
go run ./examples/demo
```

Run it twice: first run creates the config from the template, second run loads it from disk.
