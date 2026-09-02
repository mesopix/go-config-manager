package configmanager

import "fmt"

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

// Validate 按 schema 校验 data 中已存在的字段类型是否匹配。
// 缺失字段的检查和默认值补全在后续步骤中实现。
func (s Schema) Validate(data map[string]any) error {
	// 遍历 schema 每个 key
	for key, def := range s {
		val, exists := data[key]
		// data 里没有这个 key？跳过
		if !exists {
			continue // 缺失字段检查留给步骤 4
		}
		// 有？调 checkType 验证类型
		if err := checkType(key, val, def.Type); err != nil {
			// 不匹配？返回错误
			return err
		}
		// 类型匹配则继续下一个
	}
	return nil
}

// checkType 验证 val 是否匹配期望的 FieldType。
func checkType(key string, val any, expected FieldType) error {
	switch expected {
	case TypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q: expected string, got %T", key, val)
		}
	case TypeFloat:
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("field %q: expected float64, got %T", key, val)
		}
	case TypeBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q: expected bool, got %T", key, val)
		}
	}
	return nil
}
