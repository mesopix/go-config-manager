// Command demo shows how another project would use configmanager.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	configmanager "github.com/mesopix/go-config-manager"
)

// exeName returns the executable name from an os.Args[0]-style path,
// without extension (demo.exe -> demo).
//
// 库不自动识别 exe 名，是有意为之：
// 1. exe 改名会导致旧配置"丢失"（留在旧名字的目录下）
// 2. 测试二进制名字不同，会和生产配置分家
// 3. 多个二进制常要共享同一份配置
// 需要自动识别时，客户端自己搞定即可：
func exeName(arg0 string) string {
	name := filepath.Base(arg0)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func main() {
	tpl := map[string]any{"name": "demo", "port": 8080, "debug": true}

	// Always returns a config: created from tpl on first run, loaded
	// from disk afterwards. The file lives in the user config dir.
	c, err := configmanager.LoadAppConfig(exeName(os.Args[0]), tpl)
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
