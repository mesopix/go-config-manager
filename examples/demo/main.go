// Command demo shows how another project would use configmanager.
package main

import (
	"fmt"
	"log"

	configmanager "github.com/mesopix/go-config-manager"
)

func main() {
	tpl := map[string]any{"name": "demo", "port": 8080, "debug": true}

	// Always returns a config: created from tpl on first run, loaded
	// from disk afterwards. The file lives in the user config dir.
	c, err := configmanager.LoadAppConfig("demo", tpl)
	if err != nil {
		log.Fatal(err)
	}

	name, _ := c.Get("name")
	port, _ := c.Get("port")
	debug, _ := c.Get("debug")
	fmt.Printf("name=%v port=%v debug=%v\n", name, port, debug)

	// Change something and save it back to disk.
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved")
}
