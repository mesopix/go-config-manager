package configmanager

import (
	"errors"
	"fmt"
	"strings"
)

// cliCommand 是被接管的子命令名：客户端第一个参数等于它时，
// HandleCLI 接管该参数及其后的所有参数。比较区分大小写。
const cliCommand = "config"

// cliUsage 是 config 子命令的用法说明，附加在分发错误信息之后返回。
// 用 <program> 占位是因为库拿不到（也不检测）可执行文件名。
const cliUsage = `usage: <program> config --edit

  --edit    open the JSON config file for manual editing
`

// HandleCLI 接管客户端命令行中的 config 子命令。
// args 应传入 os.Args[1:]：当 args[0] 恰为 "config" 时，接管该参数及其后的
// 所有参数并返回 handled = true，客户端应跳过正常流程——出错时打印 err
// 并以非零码退出，否则直接退出（--edit 的用户反馈由库自己输出）。
// appName 与 defaultJSON 语义同 LoadAppConfig，供子命令定位或创建配置文件。
// 不是以 config 开头时返回 handled = false，不做任何事。
func HandleCLI(appName string, defaultJSON []byte, args []string) (bool, error) {
	// 空参数或第一个参数不是 config：不接管
	if len(args) == 0 || args[0] != cliCommand {
		return false, nil
	}

	// config 后没有子命令
	if len(args) == 1 {
		return true, fmt.Errorf("configmanager: missing config subcommand\n\n%s", cliUsage)
	}

	switch args[1] {
	case "--edit":
		// --edit 不接受额外参数，避免歧义
		if len(args) > 2 {
			return true, fmt.Errorf("configmanager: unexpected arguments after --edit: %s\n\n%s",
				strings.Join(args[2:], " "), cliUsage)
		}
		return true, editConfig(appName, defaultJSON)
	default:
		return true, fmt.Errorf("configmanager: unknown config subcommand %q\n\n%s", args[1], cliUsage)
	}
}

// errEditNotImplemented 是 --edit 的预留占位错误。
// 编辑器启动与编辑后校验逻辑将在后续版本实现，预留方式同 RepairAppConfig。
var errEditNotImplemented = errors.New("config edit is not implemented yet")

// editConfig 打开 appName 的配置文件供手动编辑。
// appName 与 defaultJSON 语义同 LoadAppConfig：文件缺失时需按默认模板创建。
// 本版本为预留桩。
func editConfig(appName string, defaultJSON []byte) error {
	return errEditNotImplemented
}
