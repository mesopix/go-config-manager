// 包 appconfig 是一个基于 JSON 文件的轻量级配置存储库。
//
// 实例用法：NewManager 创建一个 Manager，经 Init（可选，装配存储路径）与
// RegisterDefaults（可选，注册首运模板）完成装配，Load 返回加载好的配置
// 对象，后续所有读写都作用在这个对象上：
//
//	m := appconfig.NewManager()
//	m.Init("", "myapp", "config.json") // 可选；空参数用缺省值
//	m.RegisterDefaults(defaultJSON)    // 可选；//go:embed 的模板
//	c, err := m.Load()
//	c.Set("port", 9090)
//	c.Save()
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// defaultFileName 是配置文件名的缺省值。
const defaultFileName = "config.json"

// Manager 装配并持有一份配置：存储路径、首运模板与已加载的配置对象。
// 装配方法（Init/RegisterDefaults/Load/HandleCLI）在实例互斥锁上串行化，
// 未导出辅助方法由调用方持锁。内含 sync.Mutex：勿按值复制，始终通过指针使用。
// 指向同一路径的多个 Manager 之间没有文件锁，交叉写回时最后 Save 者胜出。
type Manager struct {
	mu       sync.Mutex
	inited   bool                   // Init 是否已成功调用
	baseDir  string                 // Init 装配的一级目录；未装配时为空
	subDir   string                 // Init 装配的二级目录；未装配时为空
	fileName string                 // Init 装配的文件名；未装配时为空
	template []byte                 // RegisterDefaults 注册并校验过的首运模板；nil 表示未注册
	cfg      *Config                // Load 成功后缓存的配置对象；nil 表示未加载
	editor   func(path string) error // --edit 编辑器启动函数；nil 时回落 launchEditor，零值 Manager 亦可用
}

// NewManager 创建一个未装配的 Manager。
func NewManager() *Manager {
	return &Manager{editor: launchEditor}
}

// Init 装配该 Manager 的存储路径，仅能在成功 Load 之前调用一次。
// 三个参数传空字符串时使用缺省值：
//   - firstDir：配置文件一级目录（完整绝对路径），缺省为 os.UserConfigDir()；
//   - secondDir：二级目录名，可含路径分隔符实现嵌套，缺省为可执行文件名
//     （不含扩展名）。注意 exe 改名会改变缺省路径，需要稳定路径时显式传入；
//   - fileName：配置文件名，缺省为 "config.json"。
//
// firstDir 必须是绝对路径；secondDir 不得为绝对路径或包含 ".." 上跳成分；
// fileName 必须是纯文件名。重复调用或成功加载后调用返回错误；
// 懒装配 Load 失败（cfg 仍为 nil）后仍可 Init。
func (m *Manager) Init(firstDir, secondDir, fileName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inited || m.cfg != nil {
		return errors.New("appconfig: Init must be called only once per Manager")
	}
	if firstDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("appconfig: resolve default base dir: %w", err)
		}
		firstDir = dir
	}
	if !filepath.IsAbs(firstDir) {
		return fmt.Errorf("appconfig: firstDir %q must be an absolute path", firstDir)
	}
	if secondDir == "" {
		secondDir = defaultSubDir()
	}
	if err := validateSubDir(secondDir); err != nil {
		return err
	}
	if fileName == "" {
		fileName = defaultFileName
	}
	if err := validateFileName(fileName); err != nil {
		return err
	}
	m.inited = true
	m.baseDir, m.subDir, m.fileName = firstDir, secondDir, fileName
	return nil
}

// validateSubDir 校验二级目录名：不得为绝对路径，Clean 后不得上跳到一级目录之外。
func validateSubDir(sub string) error {
	if filepath.IsAbs(sub) || filepath.VolumeName(sub) != "" {
		return fmt.Errorf("appconfig: secondDir %q must be a relative name", sub)
	}
	cleaned := filepath.Clean(sub)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("appconfig: secondDir %q must not traverse outside the first dir", sub)
	}
	return nil
}

// validateFileName 校验配置文件名：必须是纯文件名，不得含路径成分。
func validateFileName(name string) error {
	if name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("appconfig: fileName %q must be a plain file name", name)
	}
	return nil
}

// defaultSubDir 返回缺省二级目录：可执行文件名（不含扩展名）。
func defaultSubDir() string {
	name := filepath.Base(os.Args[0])
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// RegisterDefaults 注册首运创建配置文件所用的默认模板，仅能注册一次。
// defaultJSON 必须是合法的 JSON 对象，注册时立即校验，非法即报错；
// 校验失败不消耗"仅一次"名额，可修正后重试。
func (m *Manager) RegisterDefaults(defaultJSON []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.template != nil {
		return errors.New("appconfig: defaults already registered on this Manager")
	}
	var object map[string]any
	if err := json.Unmarshal(defaultJSON, &object); err != nil {
		return fmt.Errorf("appconfig: default template must be a JSON object: %w", err)
	}
	// JSON null 解析为 nil map 且不报错，需显式拒绝
	if object == nil {
		return errors.New("appconfig: default template must be a JSON object, got null")
	}
	m.template = defaultJSON
	return nil
}

// path 返回配置文件完整路径，不做磁盘操作。调用方需持有 m.mu。
// 未显式装配时每次调用按缺省值重算，不把结果物化进字段。
func (m *Manager) path() (string, error) {
	base, sub, name := m.baseDir, m.subDir, m.fileName
	if !m.inited {
		// 未显式装配：全部使用缺省值懒装配
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("appconfig: resolve default base dir: %w", err)
		}
		base = dir
		sub = defaultSubDir()
		name = defaultFileName
	}
	return filepath.Join(base, sub, name), nil
}

// Load 返回该 Manager 的配置对象（幂等，后续调用返回同一对象）。
// 首次调用完成路径装配并加载：文件不存在且已注册模板时按首运流程创建，
// 并从磁盘重读（保证数值类型与后续运行一致）；文件存在但无法读取或解析时
// 返回 nil 和 *CorruptConfigError，不提供默认值降级，也不覆盖磁盘上的坏文件；
// 文件不存在且未注册模板时返回错误。
// 未调用 Init 时按全缺省值装配（用户配置目录 + 可执行文件名 + config.json）。
func (m *Manager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg != nil {
		return m.cfg, nil
	}
	path, err := m.path()
	if err != nil {
		return nil, err
	}
	loaded, err := load(path)
	if err == nil {
		m.cfg = loaded
		return m.cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, &CorruptConfigError{Path: path, Err: err}
	}
	// 首次运行：按模板创建后再从磁盘重读
	if _, err := m.createFromDefaults(path); err != nil {
		return nil, err
	}
	m.cfg, err = load(path)
	if err != nil {
		return nil, err
	}
	return m.cfg, nil
}

// createFromDefaults 在 path 处按已注册模板创建配置文件（含目录）。
// 未注册模板时报错。调用方需持有 m.mu。
func (m *Manager) createFromDefaults(path string) (*Config, error) {
	if m.template == nil {
		return nil, fmt.Errorf("appconfig: config file %s does not exist and no defaults registered", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return nil, err
	}
	created, err := newConfigFromDefaults(path, m.template)
	if err != nil {
		return nil, err
	}
	if err := created.Save(); err != nil {
		return nil, err
	}
	return created, nil
}

// ensureConfigFile 返回配置文件路径，缺失且已注册模板时按首运流程创建。
// 已存在的文件（含损坏文件）原样保留——把坏文件交给编辑器手工修复
// 正是 --edit 子命令的用途。
func (m *Manager) ensureConfigFile() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := m.createFromDefaults(path); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return path, nil
}

// CorruptConfigError 表示已存在的配置文件无法读取或解析。
// 调用方应用 errors.As 识别本错误，向用户报错并退出，不得静默改用默认值。
type CorruptConfigError struct {
	Path string // 配置文件的绝对路径
	Err  error  // 原始读取或解析错误
}

// Error 实现 error 接口，信息携带配置文件路径与原始错误。
func (e *CorruptConfigError) Error() string {
	return fmt.Sprintf("config file %s is corrupt or unreadable: %v", e.Path, e.Err)
}

// Unwrap 返回原始错误，支持 errors.Is/As 链式判断。
func (e *CorruptConfigError) Unwrap() error {
	return e.Err
}

// Repair 修复该 Manager 对应的损坏配置文件。预留接口，尚未实现；
// 未来版本将基于已注册模板重建配置文件或引导用户修复。
func (m *Manager) Repair() error {
	return errRepairNotImplemented
}

// UnknownVersion 表示无法识别的数据版本号。
const UnknownVersion = "UNKNOWN"

// Config 持有以磁盘 JSON 文件为后端的配置值。
// data 存储完整的 {meta, fields} 两层结构。
// 实例方法未做并发同步：多 goroutine 共享时由调用方自行加锁。
type Config struct {
	path            string
	data            map[string]any
	declaredVersion string // 从 config.json meta.version 读取的原始版本
	resolvedVersion string // 经 schema 校验后确定的实际版本
}

// Path 返回配置文件的绝对路径。
func (c *Config) Path() string {
	return c.path
}

// DeclaredVersion 返回 config.json 中声明的数据版本号。
// 无法识别时返回 UnknownVersion。
func (c *Config) DeclaredVersion() string {
	return c.declaredVersion
}

// ResolvedVersion 返回经 schema 校验后确定的实际数据版本号。
// 未经校验或无法识别时返回 UnknownVersion。
func (c *Config) ResolvedVersion() string {
	return c.resolvedVersion
}

// ResolveVersion 在 schema 校验通过后调用，将 resolvedVersion 设为
// schema 的 meta.version。若 schemaVersion 为空则设为 UnknownVersion。
func (c *Config) ResolveVersion(schemaVersion string) {
	if schemaVersion == "" {
		c.resolvedVersion = UnknownVersion
	} else {
		c.resolvedVersion = schemaVersion
	}
}

// fields 返回 data 中 "fields" 层的 map。
// 若不存在则返回空 map（不修改 data）。
func (c *Config) fields() map[string]any {
	if f, ok := c.data["fields"].(map[string]any); ok {
		return f
	}
	return map[string]any{}
}

// Meta 返回 data 中 "meta" 层的 map（只读访问）。
func (c *Config) Meta() map[string]any {
	if m, ok := c.data["meta"].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// Get 返回 fields 层中 key 下存储的值以及它是否存在。
// JSON 数值会被反序列化为 float64。
func (c *Config) Get(key string) (any, bool) {
	v, ok := c.fields()[key]
	return v, ok
}

// Set 将 value 存储到 fields 层的 key 下。
func (c *Config) Set(key string, value any) {
	if _, ok := c.data["fields"].(map[string]any); !ok {
		c.data["fields"] = map[string]any{}
	}
	c.data["fields"].(map[string]any)[key] = value
}

// Save 以原子方式把当前值写回 JSON 文件。
// 先写入临时文件并同步到磁盘，再重命名覆盖目标文件，
// 这样即使中途崩溃也不会留下写了一半的配置。
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}

	// 在与目标文件相同的目录下创建临时文件，
	// 保证 os.Rename 可用（必须在同一文件系统上）。
	tmpFile, err := os.CreateTemp(filepath.Dir(c.path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// 后续任何步骤失败时清理临时文件。
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(b); err != nil {
		tmpFile.Close()
		return err
	}

	// Sync 确保数据在 rename 之前落盘。
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, configFileMode); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		return err
	}

	success = true
	return nil
}

// ---------- 结构体绑定 ----------

// DecodeFields 将 fields 层按 JSON tag 解码到 target（必须为指针）。
// 经 JSON 往返实现：fields 中缺失的键不会改动 target 的对应字段，
// 因此指针字段可区分"未设置"（nil）与"显式零值"（指向零值的指针）。
func (c *Config) DecodeFields(target any) error {
	encoded, err := json.Marshal(c.fields())
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

// SetFieldsFrom 将 source 按 JSON tag 编码后整体替换 fields 层，meta 层不受影响。
// source 为 nil 或无法编码为 JSON 对象时返回错误。
func (c *Config) SetFieldsFrom(source any) error {
	if source == nil {
		return errors.New("appconfig: SetFieldsFrom source must not be nil")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("appconfig: SetFieldsFrom source must encode to a JSON object")
	}
	c.data["fields"] = fields
	return nil
}

// ---------- Check / Normalize 接入 Config ----------

// Check 用 schema 校验当前 fields 层，返回校验状态。
// 等价于在 fields() 上调用 Schema.Check，但无需手动提取 map。
func (c *Config) Check(schema Schema) CheckResult {
	return schema.Check(c.fields())
}

// Normalize 按 schema 规范化当前 fields 层：补全缺失的默认值、删除多余字段。
// Valid 状态下为 no-op（直接返回 nil）；MissingDefaults / ExtraFields /
// MissingAndExtra 状态下执行规范化并写回；Invalid 状态返回错误。
func (c *Config) Normalize(schema Schema) error {
	switch c.Check(schema) {
	case Valid:
		return nil
	case MissingDefaults, ExtraFields, MissingAndExtra:
		normalized, err := schema.Normalize(c.fields())
		if err != nil {
			return err
		}
		c.data["fields"] = normalized
		return nil
	default:
		return fmt.Errorf("cannot normalize: check result is Invalid")
	}
}

// load 读取 path 处的 JSON 文件到一个新的 Config 中。
// 文件不存在视为错误。加载后自动提取 declaredVersion。
func load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{path: path, data: map[string]any{}, resolvedVersion: UnknownVersion}
	if err := json.Unmarshal(b, &c.data); err != nil {
		return nil, err
	}
	// 提取 declaredVersion
	c.declaredVersion = extractVersion(c.data)
	return c, nil
}

// extractVersion 从 {meta: {version: "..."}} 中提取版本号。
// 任何缺失或类型不匹配均返回 UnknownVersion。
func extractVersion(data map[string]any) string {
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		return UnknownVersion
	}
	v, ok := meta["version"].(string)
	if !ok || v == "" {
		return UnknownVersion
	}
	return v
}

// configDirMode 为 rwx------：仅属主可访问配置目录。
const configDirMode = 0o700

// configFileMode 为 rw-------：仅属主可读写，防止敏感配置泄露。
const configFileMode = 0o600

// newConfigFromDefaults 解析 defaultJSON 并构造 Config，不读写磁盘。
// defaultJSON 解析失败时返回 nil 和错误。
func newConfigFromDefaults(path string, defaultJSON []byte) (*Config, error) {
	var defaults map[string]any
	if err := json.Unmarshal(defaultJSON, &defaults); err != nil {
		return nil, err
	}
	config := &Config{path: path, data: defaults, resolvedVersion: UnknownVersion}
	config.declaredVersion = extractVersion(defaults)
	return config, nil
}

// errRepairNotImplemented 是预留修复接口的占位错误。
var errRepairNotImplemented = errors.New("config repair is not implemented yet")
