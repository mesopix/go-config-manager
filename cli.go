package appconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
// 所有参数并返回 shouldClose = true，客户端应跳过正常流程——出错时打印 err
// 并以非零码退出，否则直接退出（--edit 的用户反馈由库自己输出）。
// 配置文件位置与首运模板来自该 Manager 实例的装配（Init/RegisterDefaults），
// 未装配时使用缺省值。不是以 config 开头时返回 shouldClose = false，不做任何事。
// 推荐在 Load 之前调用：--edit 的修复结果可被本次运行的 Load 直接读到。
func (m *Manager) HandleCLI(args []string) (shouldClose bool, err error) {
	// 空参数或第一个参数不是 config：不接管
	if len(args) == 0 || args[0] != cliCommand {
		return false, nil
	}

	// config 后没有子命令
	if len(args) == 1 {
		return true, fmt.Errorf("appconfig: missing config subcommand\n\n%s", cliUsage)
	}

	switch args[1] {
	case "--edit":
		// --edit 不接受额外参数，避免歧义
		if len(args) > 2 {
			return true, fmt.Errorf("appconfig: unexpected arguments after --edit: %s\n\n%s",
				strings.Join(args[2:], " "), cliUsage)
		}
		return true, m.editConfig()
	default:
		return true, fmt.Errorf("appconfig: unknown config subcommand %q\n\n%s", args[1], cliUsage)
	}
}

// editConfig 打开该 Manager 的配置文件供手动编辑。
// 文件缺失且已注册模板时先按模板创建；已存在的损坏文件原样打开，
// 由用户手工修复（坏文件不被覆盖是库的设计契约）。
// 阻塞等待编辑器退出后重读文件，校验必须是 JSON 对象（与 {meta, fields}
// 形状一致），非法时返回带解析细节的错误；合法时向标准输出打印一行确认。
// 编辑器在互斥锁之外启动，避免编辑期间占用装配锁。
func (m *Manager) editConfig() error {
	path, err := m.ensureConfigFile()
	if err != nil {
		return err
	}

	// 阻塞等待用户编辑完成；editor 为 nil（如零值 Manager）时回落默认启动器
	launch := m.editor
	if launch == nil {
		launch = launchEditor
	}
	if err := launch(path); err != nil {
		return err
	}

	// 编辑器退出后重读并校验
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var object map[string]any
	if err := json.Unmarshal(b, &object); err != nil {
		return fmt.Errorf("edited config file %s is not a valid JSON object: %w", path, err)
	}

	fmt.Printf("config file edited: %s (JSON valid)\n", path)
	return nil
}

// launchEditor 以阻塞方式启动编辑器打开 path，等待其退出。
// 标准输入输出接到当前进程，保证终端编辑器（vim/nano 等）可用。
// 注意：Linux 的 xdg-open 启动 GUI 程序后立即返回，无法真正阻塞；
// 需要可靠的阻塞回退时，客户端应引导用户设置 $EDITOR。
func launchEditor(path string) error {
	program, editorArgs := resolveEditorCommand()
	cmd := exec.Command(program, append(editorArgs, path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launch editor %s: %w", program, err)
	}
	return nil
}

// resolveEditorCommand 返回编辑器命令的程序名与固定参数。
// 优先级：$VISUAL → $EDITOR → 平台默认（windows: notepad / darwin: open -W /
// 其他: xdg-open）。环境变量按空白拆分，支持 "code -w" 这类带参数的取值；
// 只含空白视为未设置。
func resolveEditorCommand() (string, []string) {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if fields := strings.Fields(os.Getenv(key)); len(fields) > 0 {
			return fields[0], fields[1:]
		}
	}
	switch runtime.GOOS {
	case "windows":
		return "notepad", nil
	case "darwin":
		return "open", []string{"-W"}
	default:
		return "xdg-open", nil
	}
}
