package configmanager

// FieldType 表示配置字段期望的 JSON 类型。
type FieldType int

const (
	TypeString FieldType = iota // JSON 字符串
	TypeFloat                   // JSON 数值（float64）
	TypeBool                    // JSON 布尔值
	TypeArray                   // JSON 数组
	TypeObject                  // JSON 对象
)

// FieldDef 描述一个配置键：期望类型、是否必填，以及可选的默认值（仅在非必填时生效）。
type FieldDef struct {
	Type     FieldType
	Required bool
	Default  any // nil 表示无默认值；仅当 Required 为 false 时有意义
}

// Schema 是以配置键名为索引的字段定义集合。
// 它有意独立于 Config，使调用方自行掌控 schema 与校验生命周期。
type Schema map[string]FieldDef
