# goconfig

可复用的 Go 配置管理模块，面向 TUI / CLI 工具（TOML 格式）。

- **模板嵌入与懒创建** — 裸二进制首次运行自动从 `//go:embed` 模板生成配置文件
- **Schema 版本管理** — 版本检测、校验、链式自动升级（v1→v2→…，未触碰字段无损保留）
- **编辑** — 全屏 TUI 菜单、`--edit`（`$VISUAL`/`$EDITOR`）、`get/set/list/del` 子命令
- **安全** — 原子写入（临时文件+rename）、迁移与编辑前写 `.bak` 备份、校验失败绝不落盘

## 快速开始

工具侧嵌入模板（带注释、含 `version` 字段）：

```toml
# config.toml — 当前 schema 模板
version = 2

[server]
host = "localhost"
port = 8080
```

```go
//go:embed config.toml
var configTOML string

m := goconfig.New(goconfig.Options{
	Path:     filepath.Join(configDir, "mytool", "config.toml"),
	Template: configTOML,
	Version:  2, // 当前 schema 版本；0 = 关闭版本管理
	Migrations: []goconfig.Migration{ // 每步只管 From → From+1
		{From: 1, Migrate: func(d *goconfig.Doc) error {
			d.Set("server.host", "localhost") // 版本号由库自动递增
			return nil
		}},
	},
	Validate: func(d *goconfig.Doc) error { /* 语义校验，返回 error 即拒绝 */ return nil },
})

doc, err := m.Load() // 无文件→写模板；旧版本→链式升级落盘；校验通过后返回
```

`Doc` 用 dot path 寻址：`doc.Get("server.port")`、`doc.Set(...)`、`doc.Delete(...)`、`doc.Keys()`。

## config 子命令

```go
if len(os.Args) > 1 && os.Args[1] == "config" {
	return m.Run(os.Args[2:]) // 可挂到任意 CLI 框架
}
```

| 命令 | 作用 |
|------|------|
| `config` | 全屏 TUI 菜单（↑/↓ 选择、enter 编辑，每次确认即校验落盘；非终端时打印 usage） |
| `config --edit` | 打开 `$VISUAL` / `$EDITOR`（Windows 回退 notepad），退出后重新解析+迁移+校验 |
| `config list` / `get <k>` | 列出 / 查询单个配置 |
| `config set <k> <v>` | 设置单值：类型跟随旧值（整型字段不会被写成字符串），新键自动猜测类型 |
| `config del <k>` | 删除单键 |

## 版本迁移

- `Options.Version` 为当前版本；加载时发现旧版本自动逐级升级（v1 直接升 v3 = v1→v2→v3）。
- 迁移在内存中完成后先跑 `Validate`，**通过才**把原文件存为 `<path>.bak` 并写入新版本。
- 任一环节失败（断链、迁移报错、校验拒绝、版本超前）：磁盘文件保持原样，下次重试。

## 已知限制

- 程序重写配置时 TOML 注释会丢失（`.bak` 保留原件）；保注释需语法树级编码器，暂未做。
- TUI 菜单只编辑已有标量键；增删键用 `config set` / `config del`。

## 示例

[`examples/demo`](examples/demo)：嵌入模板、v1→v3 迁移、校验与完整 config 命令。

```console
$ demo                                # 首次运行，自动生成模板配置
$ cp legacy-v1.toml ~/.config/demo/config.toml
$ demo                                # 一次升级 v1→v3，原件存为 config.toml.bak
$ demo config                         # TUI 菜单
$ demo config --edit                  # 打开编辑器
```
