// Command demo shows how another project would use configmanager.
package main

import (
	"fmt"
	"log"
	"os"

	configmanager "github.com/mesopix/go-config-manager"
)

func main() {
	path := "demo-config.json"

	var c *configmanager.Config
	if _, err := os.Stat(path); err == nil {
		// Later runs: load what's on disk.
		c, err = configmanager.Load(path)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("loaded", path)
	} else {
		// First run: start empty and write defaults.
		c = configmanager.New(path)
		c.Set("name", "demo")
		c.Set("port", 8080)
		c.Set("debug", true)
		if err := c.Save(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("created", path, "with defaults")
	}

	name, _ := c.Get("name")
	port, _ := c.Get("port")
	debug, _ := c.Get("debug")
	fmt.Printf("name=%v port=%v debug=%v\n", name, port, debug)
}
