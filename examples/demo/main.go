// 命令 demo 演示其他项目如何使用 appconfig。
package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/mesopix/go-config-manager"
)

// defaultConfigJSON 为嵌入的默认配置模板，首次运行时用于创建配置文件。
//
//go:embed default_config.json
var defaultConfigJSON []byte

// embeddedSchemaJSON 为嵌入的 schema 定义，编译时打包进二进制。
//
//go:embed schema.json
var embeddedSchemaJSON []byte

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
	// CLI 接管：第一个参数是 config 时，该参数及其后的所有参数交给库处理，
	// 处理完毕直接结束进程，不进入正常业务流程。
	if handled, err := appconfig.HandleCLI(exeName(os.Args[0]), defaultConfigJSON, os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// 总会返回一个配置：首次运行从嵌入的 JSON 模板创建，之后从磁盘加载。
	// 配置文件位于用户配置目录下。
	c, err := appconfig.LoadAppConfig(exeName(os.Args[0]), defaultConfigJSON)
	if err != nil {
		log.Fatal(err)
	}

	name, _ := c.Get("name")
	port, _ := c.Get("port")
	debug, _ := c.Get("debug")
	fmt.Printf("name=%v port=%v debug=%v\n", name, port, debug)

	// 从嵌入的 JSON 解析 schema（含 meta）
	sf, err := appconfig.ParseSchemaFile(embeddedSchemaJSON)
	if err != nil {
		log.Fatalf("parse embedded schema: %v", err)
	}
	schema := appconfig.Schema(sf.Fields)
	fmt.Printf("schema version: %s\n", sf.Meta.Version)

	// 提取当前配置数据用于检查
	data := map[string]any{}
	for _, key := range []string{"name", "port", "debug"} {
		if v, ok := c.Get(key); ok {
			data[key] = v
		}
	}

	result := schema.Check(data)
	fmt.Printf("check result: %d\n", result)

	// 如果可校正，则 Normalize 并写回磁盘
	switch result {
	case appconfig.MissingDefaults, appconfig.ExtraFields, appconfig.MissingAndExtra:
		normalized, err := schema.Normalize(data)
		if err != nil {
			log.Fatal(err)
		}
		for k, v := range normalized {
			c.Set(k, v)
		}
		if err := c.Save(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("normalized and saved")
	default:
		fmt.Println("no normalization needed or cannot normalize")
	}

	// 修改某个值并保存回磁盘。
	c.Set("port", 9090)
	if err := c.Save(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved")
}
