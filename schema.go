package appconfig

import (
	"encoding/json"
	"fmt"
)

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

// SchemaMeta 是 schema 文件的元数据部分，由客户端自行填充。
type SchemaMeta struct {
	Version string `json:"version"`
}

// SchemaFile 是 schema.json 的顶层结构，将元数据与字段定义分组。
type SchemaFile struct {
	Meta   SchemaMeta          `json:"meta"`
	Fields map[string]FieldDef `json:"fields"`
}

// ParseSchema 从 JSON 字节中解析出 Schema（仅提取 fields 部分）。
// 客户端可通过返回的 SchemaFile 访问 meta 信息。
func ParseSchema(data []byte) (Schema, error) {
	var sf SchemaFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	return Schema(sf.Fields), nil
}

// ParseSchemaFile 从 JSON 字节中解析出完整的 SchemaFile（含 meta）。
func ParseSchemaFile(data []byte) (*SchemaFile, error) {
	var sf SchemaFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	return &sf, nil
}

// CheckResult 表示 data 相对于 schema 的校验状态。
type CheckResult int

const (
	Valid           CheckResult = iota // 严格符合 schema
	MissingDefaults                    // 仅缺少带默认值的非必填字段
	ExtraFields                        // 仅有 schema 未定义的多余字段
	MissingAndExtra                    // 既缺带默认值的字段，又有多余字段
	Invalid                            // 必填缺失或类型不匹配
)

// Check 只读检查 data 相对于 schema 的状态，不修改 data。
func (s Schema) Check(data map[string]any) CheckResult {
	hasMissing := false
	hasExtra := false

	// 检查 schema 中定义的每个字段
	for key, def := range s {
		val, exists := data[key]
		if !exists {
			// 必填缺失 → 直接 Invalid
			if def.Required {
				return Invalid
			}
			// 非必填但有默认值 → 记为缺失
			if def.Default != nil {
				hasMissing = true
			}
			continue
		}
		// 类型不匹配 → 直接 Invalid
		if !matchType(val, def.Type) {
			return Invalid
		}
	}

	// 检查 data 中是否有 schema 未定义的多余字段
	for key := range data {
		if _, defined := s[key]; !defined {
			hasExtra = true
			break
		}
	}

	// 组合判断
	if hasMissing && hasExtra {
		return MissingAndExtra
	}
	if hasMissing {
		return MissingDefaults
	}
	if hasExtra {
		return ExtraFields
	}
	return Valid
}

// matchType 检查 val 是否匹配期望的 FieldType，不产生错误信息。
func matchType(val any, expected FieldType) bool {
	switch expected {
	case TypeString:
		_, ok := val.(string)
		return ok
	case TypeFloat:
		_, ok := val.(float64)
		return ok
	case TypeBool:
		_, ok := val.(bool)
		return ok
	case TypeArray:
		_, ok := val.([]any)
		return ok
	case TypeObject:
		_, ok := val.(map[string]any)
		return ok
	}
	return false
}

// Normalize 返回 data 的规范化副本：补全缺失的默认值，删除多余字段。
// 原 data 不会被修改。仅当 Check 结果为 MissingDefaults / ExtraFields /
// MissingAndExtra 时才能成功转换；其他状态返回 error。
func (s Schema) Normalize(data map[string]any) (map[string]any, error) {
	switch s.Check(data) {
	case MissingDefaults, ExtraFields, MissingAndExtra:
		// 可转换，继续处理
	default:
		return nil, fmt.Errorf("cannot normalize: check result is %d", s.Check(data))
	}

	result := make(map[string]any, len(s))
	// 只复制 schema 中定义的字段（自动排除多余字段）
	for key := range s {
		if val, exists := data[key]; exists {
			result[key] = val
		}
	}
	// 补全缺失的默认值
	for key, def := range s {
		if _, exists := result[key]; !exists && def.Default != nil {
			result[key] = def.Default
		}
	}
	return result, nil
}
