// 命令 demo 演示其他项目如何使用 configmanager。
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	configmanager "github.com/mesopix/go-config-manager"
)

// exeName 从 os.Args[0] 风格的路径中返回不带扩展名的可执行文件名
// （demo.exe -> demo）。
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

	// 总会返回一个配置：首次运行从 tpl 创建，之后从磁盘加载。
	// 配置文件位于用户配置目录下。
	c, err := configmanager.LoadAppConfig(exeName(os.Args[0]), tpl)
	if err != nil {
		log.Fatal(err)
	}

	name, _ := c.Get("name")
	port, _ := c.Get("port")
	debug, _ := c.Get("debug")
	fmt.Printf("name=%v port=%v debug=%v\n", name, port, debug)

	// 修改某个值并保存回磁盘。
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved")
}
