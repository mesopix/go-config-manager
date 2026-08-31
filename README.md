# go-config-manager

Tiny JSON-file-backed configuration store. Zero dependencies.

```go
c := configmanager.New("config.json")
c.Set("port", 8080)
if err := c.Save(); err != nil { ... }

c, err := configmanager.Load("config.json")
port, _ := c.Get("port")
```

Note: JSON numbers come back as `float64` from `Get`.
