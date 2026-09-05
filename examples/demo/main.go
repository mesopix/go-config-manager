// 命令 demo 演示其他项目如何使用 appconfig。
package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"

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

func main() {
	m := appconfig.NewManager()

	// 注册首运模板。
	// 必须在 HandleCLI 之前：首运机器上 config --edit 要靠模板先创建文件。
	if err := m.RegisterDefaults(defaultConfigJSON); err != nil {
		log.Fatal(err)
	}

	// CLI 接管：第一个参数是 config 时，该参数及其后的所有参数交给库处理，
	// 处理完毕直接结束进程，不进入正常业务流程。
	// 放在 Load 之前：--edit 的修复结果可被本次运行的 Load 直接读到。
	if shouldClose, err := m.HandleCLI(os.Args[1:]); shouldClose {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// 加载配置。未调用 Init 时按缺省值装配：用户配置目录 + 可执行文件名 +
	// config.json；exe 改名会改变缺省路径，需要稳定路径时先调用
	// m.Init("", "app", "config.json") 显式指定。
	c, err := m.Load()
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

	// 直接对 fields 层做检查；可校正则 Normalize 并写回磁盘
	switch result := c.Check(schema); result {
	case appconfig.MissingDefaults, appconfig.ExtraFields, appconfig.MissingAndExtra:
		if err := c.Normalize(schema); err != nil {
			log.Fatal(err)
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